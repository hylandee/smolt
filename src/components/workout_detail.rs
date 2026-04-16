use leptos::prelude::*;
use leptos_router::{components::A, hooks::use_params, params::Params};

use crate::models::ExerciseSet;
use crate::server_fns::{get_workout_detail, ToggleSetCompleted, DeleteWorkout};

#[derive(Params, PartialEq, Clone)]
struct WorkoutDetailParams {
    id: Option<i64>,
}

#[component]
pub fn WorkoutDetail() -> impl IntoView {
    let params = use_params::<WorkoutDetailParams>();
    let session_id = move || params.with(|p| p.as_ref().ok().and_then(|p| p.id).unwrap_or(0));

    let detail = Resource::new(session_id, |id| async move {
        get_workout_detail(id).await
    });

    let toggle_action = ServerAction::<ToggleSetCompleted>::new();
    let delete_action = ServerAction::<DeleteWorkout>::new();

    // After deletion navigate to workout list
    let navigate = leptos_router::hooks::use_navigate();
    Effect::new(move |_| {
        if delete_action.value().with(|v| matches!(v, Some(Ok(())))) {
            navigate("/", Default::default());
        }
    });

    view! {
        <div class="page workout-detail">
            <Suspense fallback=|| view! { <div class="loading">"Loading…"</div> }>
                {move || {
                    detail.get().map(|result| match result {
                        Err(e) => view! {
                            <div class="alert alert-error">"Error: " {e.to_string()}</div>
                        }.into_any(),
                        Ok(None) => view! {
                            <div class="alert alert-error">"Workout not found."</div>
                        }.into_any(),
                        Ok(Some(d)) => {
                            let session = d.session.clone();
                            let sets    = d.sets.clone();

                            // Group sets by exercise name
                            let mut exercises: Vec<String> = Vec::new();
                            for s in &sets {
                                if !exercises.contains(&s.exercise_name) {
                                    exercises.push(s.exercise_name.clone());
                                }
                            }

                            let title = format!("Workout {} — {}", session.workout_type,
                                session.created_at.get(..16).unwrap_or(&session.created_at).replace('T', " "));

                            view! {
                                <div>
                                    <div class="page-header">
                                        <h1>{title}</h1>
                                        <A href="/" attr:class="btn btn-secondary">"← Back"</A>
                                    </div>

                                    {session.notes.clone().map(|n| view! {
                                        <p class="session-notes-full">{n}</p>
                                    })}

                                    <div class="exercises">
                                        <For
                                            each=move || exercises.clone()
                                            key=|e| e.clone()
                                            children={
                                                let sets = sets.clone();
                                                let toggle_action = toggle_action.clone();
                                                move |exercise_name| {
                                                    let ex_sets: Vec<ExerciseSet> = sets
                                                        .iter()
                                                        .filter(|s| s.exercise_name == exercise_name)
                                                        .cloned()
                                                        .collect();
                                                    let weight = ex_sets.first().map(|s| s.weight).unwrap_or(0.0);
                                                    let title = format!("{} — {:.1} kg", exercise_name, weight);
                                                    view! {
                                                        <div class="exercise-block">
                                                            <h3>{title}</h3>
                                                            <div class="sets-row">
                                                                <For
                                                                    each=move || ex_sets.clone()
                                                                    key=|s| s.id
                                                                    children={
                                                                        let toggle_action = toggle_action.clone();
                                                                        move |s| {
                                                                            let set_id    = s.id;
                                                                            let completed = s.completed;
                                                                            let label     = format!("Set {} × {}r", s.set_number, s.reps);
                                                                            let cls = if completed { "set-btn done" } else { "set-btn failed" };
                                                                            view! {
                                                                                <button
                                                                                    class=cls
                                                                                    on:click={
                                                                                        let toggle_action = toggle_action.clone();
                                                                                        move |_| {
                                                                                            toggle_action.dispatch(ToggleSetCompleted {
                                                                                                set_id,
                                                                                                completed: !completed,
                                                                                            });
                                                                                        }
                                                                                    }
                                                                                >
                                                                                    {label}
                                                                                </button>
                                                                            }
                                                                        }
                                                                    }
                                                                />
                                                            </div>
                                                        </div>
                                                    }
                                                }
                                            }
                                        />
                                    </div>

                                    <div class="danger-zone">
                                        <button
                                            class="btn btn-danger"
                                            on:click={
                                                let delete_action = delete_action.clone();
                                                let sid = session.id;
                                                move |_| {
                                                    delete_action.dispatch(DeleteWorkout { session_id: sid });
                                                }
                                            }
                                        >
                                            "Delete workout"
                                        </button>
                                    </div>
                                </div>
                            }.into_any()
                        }
                    })
                }}
            </Suspense>
        </div>
    }
}
