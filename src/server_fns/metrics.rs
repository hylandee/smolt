use leptos::prelude::*;

use crate::models::BodyMetric;

// ── List body metrics ─────────────────────────────────────────────────────────

#[server]
pub async fn get_body_metrics() -> Result<Vec<BodyMetric>, ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    let rows = sqlx::query_as::<_, BodyMetric>(
        "SELECT id, recorded_at, weight_kg, notes \
         FROM body_metrics \
         ORDER BY recorded_at DESC \
         LIMIT 200",
    )
    .fetch_all(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(rows)
}

// ── Add a body metric entry ───────────────────────────────────────────────────

#[server]
pub async fn add_body_metric(
    weight_kg: f64,
    notes:     String,
) -> Result<i64, ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    if weight_kg <= 0.0 {
        return Err(ServerFnError::new("weight_kg must be positive"));
    }

    let notes_opt: Option<String> = if notes.trim().is_empty() {
        None
    } else {
        Some(notes)
    };

    let id: i64 = sqlx::query_scalar(
        "INSERT INTO body_metrics (weight_kg, notes) VALUES (?, ?) RETURNING id",
    )
    .bind(weight_kg)
    .bind(notes_opt)
    .fetch_one(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(id)
}

// ── Delete a body metric entry ────────────────────────────────────────────────

#[server]
pub async fn delete_body_metric(metric_id: i64) -> Result<(), ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    sqlx::query("DELETE FROM body_metrics WHERE id = ?")
        .bind(metric_id)
        .execute(&pool)
        .await
        .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(())
}

// ── Bodyweight history for the progress chart ─────────────────────────────────

#[server]
pub async fn get_bodyweight_history() -> Result<Vec<(String, f64)>, ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    let rows: Vec<(String, f64)> = sqlx::query_as(
        "SELECT recorded_at, weight_kg \
         FROM body_metrics \
         ORDER BY recorded_at ASC \
         LIMIT 200",
    )
    .fetch_all(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(rows)
}
