use leptos::form::ActionForm;
use leptos::prelude::*;

use crate::server_fns::{
    get_body_metrics, AddBodyMetric, DeleteBodyMetric,
};

#[component]
pub fn BodyMetrics() -> impl IntoView {
    let metrics  = Resource::new(|| (), |_| async { get_body_metrics().await });
    let add_act  = ServerAction::<AddBodyMetric>::new();
    let del_act  = ServerAction::<DeleteBodyMetric>::new();

    // Refresh the list after add / delete
    Effect::new(move |_| {
        if add_act.value().with(|v| matches!(v, Some(Ok(_)))) {
            metrics.refetch();
        }
    });
    Effect::new(move |_| {
        if del_act.value().with(|v| matches!(v, Some(Ok(())))) {
            metrics.refetch();
        }
    });

    view! {
        <div class="page body-metrics">
            <div class="page-header">
                <h1>"Body Metrics"</h1>
            </div>

            // Add form
            <div class="card">
                <h2>"Log entry"</h2>
                {move || add_act.value().get().and_then(|r| r.err()).map(|e| view! {
                    <div class="alert alert-error">{e.to_string()}</div>
                })}
                <div class="form form-inline">
                    <ActionForm action=add_act>
                        <div class="form-group">
                            <label class="form-label" for="weight_kg">"Weight (kg)"</label>
                            <input
                                type="number"
                                id="weight_kg"
                                name="weight_kg"
                                min="0"
                                step="0.1"
                                placeholder="75.0"
                                required
                                class="input"
                            />
                        </div>
                        <div class="form-group">
                            <label class="form-label" for="notes">"Notes (optional)"</label>
                            <input
                                type="text"
                                id="notes"
                                name="notes"
                                placeholder="morning, after workout…"
                                class="input"
                            />
                        </div>
                        <button type="submit" class="btn btn-primary" disabled=move || add_act.pending().get()>
                            {move || if add_act.pending().get() { "Saving…" } else { "Add" }}
                        </button>
                    </ActionForm>
                </div>
            </div>

            // Metric list
            <Suspense fallback=|| view! { <div class="loading">"Loading…"</div> }>
                {move || {
                    metrics.get().map(|result| match result {
                        Err(e) => view! {
                            <div class="alert alert-error">"Error: " {e.to_string()}</div>
                        }.into_any(),
                        Ok(list) if list.is_empty() => view! {
                            <p class="empty-state">"No entries yet."</p>
                        }.into_any(),
                        Ok(list) => view! {
                            <table class="metrics-table">
                                <thead>
                                    <tr>
                                        <th>"Date"</th>
                                        <th>"Weight (kg)"</th>
                                        <th>"Notes"</th>
                                        <th></th>
                                    </tr>
                                </thead>
                                <tbody>
                                    <For
                                        each=move || list.clone()
                                        key=|m| m.id
                                        children={
                                            let del_act = del_act.clone();
                                            move |m| {
                                                let metric_id = m.id;
                                                let date = m.recorded_at.get(..16).unwrap_or(&m.recorded_at).replace('T', " ").to_string();
                                                view! {
                                                    <tr>
                                                        <td>{date}</td>
                                                        <td>{format!("{:.1}", m.weight_kg)}</td>
                                                        <td>{m.notes.clone().unwrap_or_default()}</td>
                                                        <td>
                                                            <button
                                                                class="btn-icon btn-danger"
                                                                on:click={
                                                                    let del_act = del_act.clone();
                                                                    move |_| {
                                                                        del_act.dispatch(DeleteBodyMetric { metric_id });
                                                                    }
                                                                }
                                                            >"✕"</button>
                                                        </td>
                                                    </tr>
                                                }
                                            }
                                        }
                                    />
                                </tbody>
                            </table>
                        }.into_any(),
                    })
                }}
            </Suspense>
        </div>
    }
}
