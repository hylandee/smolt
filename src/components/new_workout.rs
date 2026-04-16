use leptos::form::ActionForm;
use leptos::prelude::*;

use crate::models::{built_in_templates, workout_exercises};
use crate::server_fns::StartWorkout;

#[component]
pub fn NewWorkout() -> impl IntoView {
    let templates = built_in_templates();
    let first_template = templates.first().map(|t| t.name.to_string()).unwrap_or_default();

    // Selected template / workout name (user can type anything)
    let (workout_name, set_workout_name) = signal(first_template);
    let (weight_unit, set_weight_unit) = signal("kg".to_string());

    let action   = ServerAction::<StartWorkout>::new();
    let navigate = leptos_router::hooks::use_navigate();

    Effect::new(move |_| {
        if let Some(Ok(id)) = action.value().get() {
            navigate(&format!("/workouts/{}", id), Default::default());
        }
    });

    // Exercises from the selected built-in template (empty for custom names)
    let exercises = move || workout_exercises(&workout_name.get());

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
                    // ── Workout name ──────────────────────────────────────────
                    <div class="form-group">
                        <label class="form-label">"Workout name"</label>
                        // Quick-select built-in templates
                        <div class="btn-group">
                            {
                                let templates = built_in_templates();
                                templates.into_iter().map(|t| {
                                    let name = t.name.to_string();
                                    let name2 = name.clone();
                                    view! {
                                        <button
                                            type="button"
                                            class=move || {
                                                if workout_name.get() == name { "btn btn-selected" } else { "btn" }
                                            }
                                            on:click={
                                                let name2 = name2.clone();
                                                move |_| set_workout_name.set(name2.clone())
                                            }
                                        >{t.name}</button>
                                    }
                                }).collect_view()
                            }
                        </div>
                        // Editable name field (pre-filled from template, can be customised)
                        <input
                            type="text"
                            name="workout_name"
                            class="input"
                            placeholder="e.g. Stronglifts A, My Custom Day"
                            value=move || workout_name.get()
                            on:input=move |ev| {
                                set_workout_name.set(event_target_value(&ev));
                            }
                        />
                    </div>

                    // ── Weight unit ───────────────────────────────────────────
                    <div class="form-group">
                        <label class="form-label">"Weight unit"</label>
                        <div class="btn-group">
                            <button
                                type="button"
                                class=move || if weight_unit.get() == "kg" { "btn btn-selected" } else { "btn" }
                                on:click=move |_| set_weight_unit.set("kg".into())
                            >"kg"</button>
                            <button
                                type="button"
                                class=move || if weight_unit.get() == "lb" { "btn btn-selected" } else { "btn" }
                                on:click=move |_| set_weight_unit.set("lb".into())
                            >"lb"</button>
                        </div>
                        <input type="hidden" name="weight_unit" value=move || weight_unit.get()/>
                    </div>

                    // ── Working weights ───────────────────────────────────────
                    <div class="form-group">
                        <label class="form-label">
                            "Working weights (" {move || weight_unit.get()} ")"
                        </label>
                        <div class="exercise-inputs">
                            {move || {
                                let exs = exercises();
                                let names = ["squat_weight", "second_weight", "third_weight"];
                                if exs.is_empty() {
                                    // Custom workout — show 3 generic weight slots
                                    names.iter().enumerate().map(|(i, &field_name)| {
                                        let label = format!("Exercise {} weight", i + 1);
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
                                } else {
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
                                }
                            }}
                        </div>
                    </div>

                    // ── Notes ─────────────────────────────────────────────────
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
