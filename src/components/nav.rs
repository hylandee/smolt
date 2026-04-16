use leptos::prelude::*;
use leptos_router::components::A;

#[component]
pub fn Nav() -> impl IntoView {
    view! {
        <nav class="navbar">
            <div class="navbar-brand">
                <A href="/" attr:class="brand-link">"💪 Smolt"</A>
            </div>
            <ul class="navbar-menu">
                <li><A href="/">"Workouts"</A></li>
                <li><A href="/workouts/new">"New Workout"</A></li>
                <li><A href="/metrics">"Body Metrics"</A></li>
                <li><A href="/charts">"Progress"</A></li>
            </ul>
        </nav>
    }
}
