use leptos::prelude::*;
use leptos_meta::*;
use leptos_router::{
    components::{Route, Router, Routes},
    path,
};

use crate::components::{
    BodyMetrics, Charts, Nav, NewWorkout, WorkoutDetail, WorkoutList,
};

/// Top-level application component.
///
/// During SSR this is rendered into the body by the shell; on the client
/// it is hydrated in-place.
#[component]
pub fn App() -> impl IntoView {
    provide_meta_context();

    view! {
        <Stylesheet id="smolt" href="/pkg/smolt.css"/>
        <Title text="Smolt — StrongLifts Tracker"/>
        <Meta charset="utf-8"/>
        <Meta name="viewport" content="width=device-width, initial-scale=1"/>

        <Nav/>

        <main class="main-content">
            <Router>
                <Routes fallback=|| view! {
                    <div class="page">
                        <h1>"404 — Page Not Found"</h1>
                        <a href="/">"← Go home"</a>
                    </div>
                }>
                    <Route path=path!("/")                 view=WorkoutList/>
                    <Route path=path!("/workouts/new")     view=NewWorkout/>
                    <Route path=path!("/workouts/:id")     view=WorkoutDetail/>
                    <Route path=path!("/metrics")          view=BodyMetrics/>
                    <Route path=path!("/charts")           view=Charts/>
                </Routes>
            </Router>
        </main>
    }
}
