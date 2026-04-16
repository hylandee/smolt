use serde::{Deserialize, Serialize};

/// A single recorded workout session (Workout A or B).
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[cfg_attr(feature = "ssr", derive(sqlx::FromRow))]
pub struct WorkoutSession {
    pub id:           i64,
    pub workout_type: String,         // "A" or "B"
    pub created_at:   String,         // ISO-8601 text
    pub notes:        Option<String>,
}

/// One set within a workout session.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[cfg_attr(feature = "ssr", derive(sqlx::FromRow))]
pub struct ExerciseSet {
    pub id:            i64,
    pub session_id:    i64,
    pub exercise_name: String,
    pub set_number:    i32,
    pub reps:          i32,
    pub weight:        f64,
    pub completed:     bool,
}

/// Body-weight / metric log entry.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[cfg_attr(feature = "ssr", derive(sqlx::FromRow))]
pub struct BodyMetric {
    pub id:          i64,
    pub recorded_at: String,
    pub weight_kg:   f64,
    pub notes:       Option<String>,
}

/// Aggregated view returned for the detail page.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct WorkoutDetail {
    pub session: WorkoutSession,
    pub sets:    Vec<ExerciseSet>,
}

/// A single data point for the lift-progress chart.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct LiftHistoryPoint {
    pub date:   String,
    pub weight: f64,
}

// ── Workout templates ────────────────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ExerciseTemplate {
    pub name: String,
    pub sets: i32,
    pub reps: i32,
}

pub fn workout_exercises(workout_type: &str) -> Vec<ExerciseTemplate> {
    match workout_type {
        "A" => vec![
            ExerciseTemplate { name: "Squat".into(),        sets: 5, reps: 5 },
            ExerciseTemplate { name: "Bench Press".into(),  sets: 5, reps: 5 },
            ExerciseTemplate { name: "Barbell Row".into(),  sets: 5, reps: 5 },
        ],
        "B" => vec![
            ExerciseTemplate { name: "Squat".into(),          sets: 5, reps: 5 },
            ExerciseTemplate { name: "Overhead Press".into(), sets: 5, reps: 5 },
            ExerciseTemplate { name: "Deadlift".into(),       sets: 1, reps: 5 },
        ],
        _ => vec![],
    }
}

pub const ALL_EXERCISES: &[&str] = &[
    "Squat",
    "Bench Press",
    "Barbell Row",
    "Overhead Press",
    "Deadlift",
];
