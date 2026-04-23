#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="smolt"
REPO_DIR="/root/dev/smolt"
MODE="setup"

usage() {
  cat <<'USAGE'
Usage:
  sudo bash scripts/setup_service.sh [options]

Options:
  --repo-dir PATH        Repository directory (default: /root/dev/smolt)
  --service-name NAME    systemd service name (default: smolt)
  --restart-latest       Rebuild from local repo code and stop/start service only
  --help                 Show this help

Examples:
  sudo bash scripts/setup_service.sh
  sudo bash scripts/setup_service.sh --restart-latest
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-dir)
      REPO_DIR="$2"
      shift 2
      ;;
    --service-name)
      SERVICE_NAME="$2"
      shift 2
      ;;
    --restart-latest)
      MODE="restart-latest"
      shift
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ "$EUID" -ne 0 ]]; then
  echo "Run as root (use sudo)."
  exit 1
fi

if [[ ! -d "$REPO_DIR" ]]; then
  echo "Repo directory not found: $REPO_DIR"
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required but not installed."
  echo "Install it first, then rerun this script."
  exit 1
fi

echo "Building application in $REPO_DIR..."
cd "$REPO_DIR"
go mod download
go build -o stronglifts ./cmd/stronglifts

if [[ "$MODE" == "restart-latest" ]]; then
  echo "Restart mode: no git pull is performed by this script."
  if [[ ! -f "/etc/systemd/system/${SERVICE_NAME}.service" ]]; then
    echo "Service file not found: /etc/systemd/system/${SERVICE_NAME}.service"
    echo "Run setup mode first to create the service unit."
    exit 1
  fi

  echo "Stopping service: $SERVICE_NAME"
  systemctl stop "$SERVICE_NAME" || true
  echo "Starting service: $SERVICE_NAME"
  systemctl start "$SERVICE_NAME"

  echo "Done. Service status:"
  systemctl --no-pager --full status "$SERVICE_NAME" | sed -n '1,25p'
  echo
  echo "Tail logs with: journalctl -u $SERVICE_NAME -f"
  exit 0
fi

UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

echo "Writing systemd unit: $UNIT_PATH"
cat >"$UNIT_PATH" <<EOF2
[Unit]
Description=Smolt workout tracker
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$REPO_DIR
ExecStart=$REPO_DIR/stronglifts
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF2

echo "Reloading and starting systemd service..."
systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

echo "Done. Service status:"
systemctl --no-pager --full status "$SERVICE_NAME" | sed -n '1,25p'

echo
echo "Tail logs with: journalctl -u $SERVICE_NAME -f"
