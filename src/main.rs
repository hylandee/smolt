#[cfg(feature = "ssr")]
#[tokio::main]
async fn main() {
    use axum::Router;
    use leptos::config::get_configuration;
    use leptos::hydration::{AutoReload, HydrationScripts};
    use leptos::prelude::*;
    use leptos_axum::{file_and_error_handler, generate_route_list, LeptosRoutes};
    use leptos_meta::MetaTags;
    use smolt::app::App;
    use sqlx::sqlite::SqlitePoolOptions;
    use std::net::SocketAddr;
    use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

    // ── Load .env (optional) ──────────────────────────────────────────────
    let _ = dotenvy::dotenv();

    // ── Structured logging ────────────────────────────────────────────────
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "smolt=info,axum=info,tower_http=info".into()),
        )
        .with(tracing_subscriber::fmt::layer())
        .init();

    // ── Database ──────────────────────────────────────────────────────────
    let db_url = std::env::var("DATABASE_URL")
        .or_else(|_| std::env::var("DB_PATH").map(|p| format!("sqlite:{}", p)))
        .unwrap_or_else(|_| "sqlite:smolt.db".to_string());

    tracing::info!(db_url = %db_url, "Connecting to database");

    let pool = SqlitePoolOptions::new()
        .max_connections(5)
        .connect(&db_url)
        .await
        .expect("Failed to connect to SQLite database");

    sqlx::migrate!("./migrations")
        .run(&pool)
        .await
        .expect("Failed to run database migrations");

    tracing::info!("Database migrations complete");

    // ── Leptos config ─────────────────────────────────────────────────────
    let conf = get_configuration(Some("Cargo.toml")).unwrap();
    let mut leptos_options = conf.leptos_options;

    // Allow HOST / PORT env vars to override the leptos defaults
    let host = std::env::var("HOST").unwrap_or_else(|_| "127.0.0.1".to_string());
    let port: u16 = std::env::var("PORT")
        .ok()
        .and_then(|p| p.parse().ok())
        .unwrap_or(3000);
    let addr: SocketAddr = format!("{}:{}", host, port)
        .parse()
        .expect("Invalid HOST/PORT combination");
    leptos_options.site_addr = addr;

    // ── App state ─────────────────────────────────────────────────────────
    #[derive(Clone, axum::extract::FromRef)]
    struct AppState {
        leptos_options: leptos::config::LeptosOptions,
        pool: sqlx::SqlitePool,
    }

    let app_state = AppState {
        leptos_options: leptos_options.clone(),
        pool: pool.clone(),
    };

    // ── HTML shell (full page wrapper) ────────────────────────────────────
    fn shell(options: leptos::config::LeptosOptions) -> impl IntoView {
        view! {
            <!DOCTYPE html>
            <html lang="en">
                <head>
                    <meta charset="utf-8"/>
                    <meta name="viewport" content="width=device-width, initial-scale=1"/>
                    <AutoReload options=options.clone() />
                    <HydrationScripts options=options/>
                    <MetaTags/>
                </head>
                <body>
                    <App/>
                </body>
            </html>
        }
    }

    let routes = generate_route_list({
        let opts = leptos_options.clone();
        move || shell(opts.clone())
    });

    // ── Axum router ───────────────────────────────────────────────────────
    let app = Router::new()
        .leptos_routes_with_context(
            &app_state,
            routes,
            {
                let pool = pool.clone();
                move || provide_context(pool.clone())
            },
            {
                let opts = leptos_options.clone();
                move || shell(opts.clone())
            },
        )
        .fallback(file_and_error_handler::<AppState, _>(shell))
        .with_state(app_state);

    tracing::info!(addr = %addr, "Listening");
    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .expect("Failed to bind address");
    axum::serve(listener, app.into_make_service())
        .await
        .expect("Server error");
}

#[cfg(not(feature = "ssr"))]
fn main() {}
