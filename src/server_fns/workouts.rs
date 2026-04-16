use leptos::prelude::*;

use crate::models::*;

// ── List all workout sessions ─────────────────────────────────────────────────

#[server]
pub async fn get_workout_sessions() -> Result<Vec<WorkoutSession>, ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    let rows = sqlx::query_as::<_, WorkoutSession>(
        "SELECT id, workout_name, created_at, notes \
         FROM workout_sessions \
         ORDER BY created_at DESC \
         LIMIT 200",
    )
    .fetch_all(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(rows)
}

// ── Get a single workout with its sets ────────────────────────────────────────

#[server]
pub async fn get_workout_detail(session_id: i64) -> Result<Option<WorkoutDetail>, ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    let session = sqlx::query_as::<_, WorkoutSession>(
        "SELECT id, workout_name, created_at, notes \
         FROM workout_sessions WHERE id = ?",
    )
    .bind(session_id)
    .fetch_optional(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    let Some(session) = session else {
        return Ok(None);
    };

    let sets = sqlx::query_as::<_, ExerciseSet>(
        "SELECT id, session_id, exercise_name, set_number, reps, weight, weight_unit, completed \
         FROM exercise_sets \
         WHERE session_id = ? \
         ORDER BY exercise_name, set_number",
    )
    .bind(session_id)
    .fetch_all(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(Some(WorkoutDetail { session, sets }))
}

// ── Start a new workout session ───────────────────────────────────────────────
//
// `workout_name` can be anything — "Stronglifts A", "Stronglifts B", or any
// user-defined name.  The exercises are looked up from the built-in templates
// when the name matches; a future API can POST arbitrary exercises instead.

#[server]
pub async fn start_workout(
    workout_name:  String,
    squat_weight:  f64,
    second_weight: f64,
    third_weight:  f64,
    weight_unit:   String,  // "kg" or "lb" — applies to all exercises in this session
    notes:         String,
) -> Result<i64, ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    if workout_name.trim().is_empty() {
        return Err(ServerFnError::new("Workout name cannot be empty"));
    }
    let unit = if weight_unit.trim().eq_ignore_ascii_case("lb") {
        "lb"
    } else {
        "kg"
    };

    let exercises = workout_exercises(&workout_name);
    let weights = [squat_weight, second_weight, third_weight];

    let notes_opt: Option<String> = if notes.trim().is_empty() {
        None
    } else {
        Some(notes)
    };

    let session_id: i64 = sqlx::query_scalar(
        "INSERT INTO workout_sessions (workout_name, notes) VALUES (?, ?) RETURNING id",
    )
    .bind(&workout_name)
    .bind(notes_opt)
    .fetch_one(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    for (i, ex) in exercises.iter().enumerate() {
        let w = weights.get(i).copied().unwrap_or(0.0);
        for set_num in 1..=ex.sets {
            sqlx::query(
                "INSERT INTO exercise_sets \
                 (session_id, exercise_name, set_number, reps, weight, weight_unit, completed) \
                 VALUES (?, ?, ?, ?, ?, ?, 1)",
            )
            .bind(session_id)
            .bind(&ex.name)
            .bind(set_num)
            .bind(ex.reps)
            .bind(w)
            .bind(unit)
            .execute(&pool)
            .await
            .map_err(|e| ServerFnError::new(e.to_string()))?;
        }
    }

    Ok(session_id)
}

// ── Toggle set completed/failed ────────────────────────────────────────────────

#[server]
pub async fn toggle_set_completed(
    set_id:    i64,
    completed: bool,
) -> Result<(), ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    sqlx::query("UPDATE exercise_sets SET completed = ? WHERE id = ?")
        .bind(completed)
        .bind(set_id)
        .execute(&pool)
        .await
        .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(())
}

// ── Update set weight ─────────────────────────────────────────────────────────

#[server]
pub async fn update_set_weight(
    set_id:      i64,
    weight:      f64,
    weight_unit: String,
) -> Result<(), ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    let unit = if weight_unit.trim().eq_ignore_ascii_case("lb") { "lb" } else { "kg" };

    sqlx::query("UPDATE exercise_sets SET weight = ?, weight_unit = ? WHERE id = ?")
        .bind(weight)
        .bind(unit)
        .bind(set_id)
        .execute(&pool)
        .await
        .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(())
}

// ── Delete workout ─────────────────────────────────────────────────────────────

#[server]
pub async fn delete_workout(session_id: i64) -> Result<(), ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    sqlx::query("DELETE FROM workout_sessions WHERE id = ?")
        .bind(session_id)
        .execute(&pool)
        .await
        .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(())
}

// ── Lift history for charts ────────────────────────────────────────────────────

#[server]
pub async fn get_lift_history(
    exercise: String,
) -> Result<Vec<LiftHistoryPoint>, ServerFnError> {
    let pool = use_context::<sqlx::SqlitePool>()
        .ok_or_else(|| ServerFnError::new("DB pool not in context"))?;

    // Return the maximum weight per session for this exercise (completed sets only).
    // weight_unit is taken from the first row of each session group.
    let rows: Vec<(String, f64, String)> = sqlx::query_as(
        "SELECT ws.created_at, MAX(es.weight) as weight, es.weight_unit \
         FROM exercise_sets es \
         JOIN workout_sessions ws ON es.session_id = ws.id \
         WHERE es.exercise_name = ? AND es.completed = 1 \
         GROUP BY es.session_id, ws.created_at, es.weight_unit \
         ORDER BY ws.created_at ASC \
         LIMIT 100",
    )
    .bind(&exercise)
    .fetch_all(&pool)
    .await
    .map_err(|e| ServerFnError::new(e.to_string()))?;

    Ok(rows
        .into_iter()
        .map(|(date, weight, weight_unit)| LiftHistoryPoint { date, weight, weight_unit })
        .collect())
}
