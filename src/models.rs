use serde::{Deserialize, Serialize};

// ── Weight unit ───────────────────────────────────────────────────────────────

/// The unit used for a weight measurement.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum WeightUnit {
    #[default]
    Kg,
    Lb,
}

impl WeightUnit {
    pub fn as_str(&self) -> &'static str {
        match self {
            WeightUnit::Kg => "kg",
            WeightUnit::Lb => "lb",
        }
    }
}

impl std::fmt::Display for WeightUnit {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

impl From<String> for WeightUnit {
    fn from(s: String) -> Self {
        match s.to_lowercase().as_str() {
            "lb" | "lbs" => WeightUnit::Lb,
            _ => WeightUnit::Kg,
        }
    }
}

// ── DB models ─────────────────────────────────────────────────────────────────

/// A single recorded workout session.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[cfg_attr(feature = "ssr", derive(sqlx::FromRow))]
pub struct WorkoutSession {
    pub id:           i64,
    pub workout_name: String,  // user-defined name, e.g. "Stronglifts A"
    pub created_at:   String,  // ISO-8601 text
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
    pub weight_unit:   String,  // "kg" or "lb"
    pub completed:     bool,
}

/// Body-weight / metric log entry.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[cfg_attr(feature = "ssr", derive(sqlx::FromRow))]
pub struct BodyMetric {
    pub id:          i64,
    pub recorded_at: String,
    pub weight:      f64,
    pub weight_unit: String,  // "kg" or "lb"
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
    pub date:        String,
    pub weight:      f64,
    pub weight_unit: String,
}

// ── Exercise templates ────────────────────────────────────────────────────────

/// Describes one exercise in a workout template.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ExerciseTemplate {
    pub name: String,
    pub sets: i32,
    pub reps: i32,
}

/// A named workout template (e.g. "Stronglifts A").
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct WorkoutTemplate {
    pub name:      &'static str,
    pub exercises: Vec<ExerciseTemplate>,
}

/// Returns the built-in workout templates.  These are suggestions; users can
/// create workouts with any name and custom exercise selections.
pub fn built_in_templates() -> Vec<WorkoutTemplate> {
    vec![
        WorkoutTemplate {
            name: "Stronglifts A",
            exercises: vec![
                ExerciseTemplate { name: "Squat".into(),        sets: 5, reps: 5 },
                ExerciseTemplate { name: "Bench Press".into(),  sets: 5, reps: 5 },
                ExerciseTemplate { name: "Barbell Row".into(),  sets: 5, reps: 5 },
            ],
        },
        WorkoutTemplate {
            name: "Stronglifts B",
            exercises: vec![
                ExerciseTemplate { name: "Squat".into(),          sets: 5, reps: 5 },
                ExerciseTemplate { name: "Overhead Press".into(), sets: 5, reps: 5 },
                ExerciseTemplate { name: "Deadlift".into(),       sets: 1, reps: 5 },
            ],
        },
    ]
}

/// Look up exercises for a named template.  Returns an empty vec if the name
/// does not match any built-in template (caller builds custom exercises instead).
pub fn workout_exercises(workout_name: &str) -> Vec<ExerciseTemplate> {
    built_in_templates()
        .into_iter()
        .find(|t| t.name == workout_name)
        .map(|t| t.exercises)
        .unwrap_or_default()
}

/// All known exercises (used for chart lift-selector and auto-complete).
pub const ALL_EXERCISES: &[&str] = &[
    "Squat",
    "Bench Press",
    "Barbell Row",
    "Overhead Press",
    "Deadlift",
    "Pullups",
    "Chinups",
    "Pushups",
    "Lunges",
];
