use leptos::prelude::*;
use leptos_router::components::A;

use crate::server_fns::get_workout_sessions;

#[component]
pub fn WorkoutList() -> impl IntoView {
    let sessions = Resource::new(|| (), |_| async { get_workout_sessions().await });

    view! {
        <div class="page workout-list">
            <div class="page-header">
                <h1>"Workouts"</h1>
                <A href="/workouts/new" attr:class="btn btn-primary">"+ New Workout"</A>
            </div>
            <Suspense fallback=|| view! { <div class="loading">"Loading workouts…"</div> }>
                {move || {
                    sessions.get().map(|result| match result {
                        Err(e) => view! {
                            <div class="alert alert-error">"Error: " {e.to_string()}</div>
                        }.into_any(),
                        Ok(list) if list.is_empty() => view! {
                            <div class="empty-state">
                                <p>"No workouts yet."</p>
                                <A href="/workouts/new" attr:class="btn btn-primary">"Start your first workout"</A>
                            </div>
                        }.into_any(),
                        Ok(list) => view! {
                            <div class="session-grid">
                                <For
                                    each=move || list.clone()
                                    key=|s| s.id
                                    children=move |s| {
                                        let href = format!("/workouts/{}", s.id);
                                        let label = format!("Workout {}", s.workout_type);
                                        let date = format_date(&s.created_at);
                                        let badge = format!("badge badge-{}", s.workout_type.to_lowercase());
                                        let notes = s.notes.clone();
                                        view! {
                                            <a href=href class="session-card">
                                                <span class=badge>
                                                    {label}
                                                </span>
                                                <span class="session-date">{date}</span>
                                                {notes.map(|n| view! {
                                                    <span class="session-notes">{n}</span>
                                                })}
                                            </a>
                                        }
                                    }
                                />
                            </div>
                        }.into_any(),
                    })
                }}
            </Suspense>
        </div>
    }
}

/// Trim the seconds from an ISO-8601 datetime for display.
fn format_date(dt: &str) -> String {
    dt.get(..16).unwrap_or(dt).replace('T', " ")
}
