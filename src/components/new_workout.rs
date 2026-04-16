use leptos::form::ActionForm;
use leptos::prelude::*;

use crate::models::workout_exercises;
use crate::server_fns::StartWorkout;

#[component]
pub fn NewWorkout() -> impl IntoView {
    // Track selected workout type so we can show the right exercise labels.
    let (wtype, set_wtype) = signal("A".to_string());

    let action   = ServerAction::<StartWorkout>::new();
    let navigate = leptos_router::hooks::use_navigate();

    // After the server action completes successfully, redirect to the detail page.
    Effect::new(move |_| {
        if let Some(Ok(id)) = action.value().get() {
            navigate(&format!("/workouts/{}", id), Default::default());
        }
    });

    let exercises = move || workout_exercises(&wtype.get());

    view! {
        <div class="page new-workout">
            <div class="page-header">
                <h1>"New Workout"</h1>
            </div>

            {move || action.value().get().and_then(|r| r.err()).map(|e| view! {
                <div class="alert alert-error">{e.to_string()}</div>
            })}

            <div class="form">
                <ActionForm action=action>
                    // Workout type toggle
                    <div class="form-group">
                        <label class="form-label">"Workout type"</label>
                        <div class="btn-group">
                            <button
                                type="button"
                                class=move || if wtype.get() == "A" { "btn btn-selected" } else { "btn" }
                                on:click=move |_| set_wtype.set("A".into())
                            >"A (Squat / Bench / Row)"</button>
                            <button
                                type="button"
                                class=move || if wtype.get() == "B" { "btn btn-selected" } else { "btn" }
                                on:click=move |_| set_wtype.set("B".into())
                            >"B (Squat / OHP / Deadlift)"</button>
                        </div>
                        // Hidden field that goes into the form payload
                        <input type="hidden" name="workout_type" value=move || wtype.get()/>
                    </div>

                    // Weight inputs — one per exercise
                    <div class="form-group">
                        <label class="form-label">"Working weights (kg)"</label>
                        <div class="exercise-inputs">
                            {move || {
                                let exs = exercises();
                                let names = ["squat_weight", "second_weight", "third_weight"];
                                exs.into_iter().enumerate().map(|(i, ex)| {
                                    let field_name = names[i];
                                    let label = format!("{} — {} × {}r", ex.name, ex.sets, ex.reps);
                                    view! {
                                        <div class="exercise-input-row">
                                            <label>{label}</label>
                                            <input
                                                type="number"
                                                name=field_name
                                                min="0"
                                                step="2.5"
                                                placeholder="0.0"
                                                class="input"
                                            />
                                        </div>
                                    }
                                }).collect_view()
                            }}
                        </div>
                    </div>

                    <div class="form-group">
                        <label class="form-label" for="notes">"Notes (optional)"</label>
                        <input
                            type="text"
                            id="notes"
                            name="notes"
                            placeholder="e.g. felt strong today"
                            class="input"
                        />
                    </div>

                    <button type="submit" class="btn btn-primary" disabled=move || action.pending().get()>
                        {move || if action.pending().get() { "Saving…" } else { "Start Workout" }}
                    </button>
                </ActionForm>
            </div>
        </div>
    }
}
