use leptos::prelude::*;

use crate::models::{LiftHistoryPoint, ALL_EXERCISES};
use crate::server_fns::{get_bodyweight_history, get_lift_history};

// ── Charts page ────────────────────────────────────────────────────────────────

#[component]
pub fn Charts() -> impl IntoView {
    let (selected_lift, set_selected_lift) =
        signal(ALL_EXERCISES.first().copied().unwrap_or("Squat").to_string());

    let lift_history = Resource::new(
        move || selected_lift.get(),
        |ex| async move { get_lift_history(ex).await },
    );

    let bw_history = Resource::new(
        || (),
        |_| async move { get_bodyweight_history().await },
    );

    view! {
        <div class="page charts">
            <div class="page-header">
                <h1>"Progress Charts"</h1>
            </div>

            // ── Lift chart ──────────────────────────────────────────────────
            <div class="chart-section">
                <h2>"Lift Progress"</h2>
                <div class="lift-selector">
                    {ALL_EXERCISES.iter().map(|ex| {
                        let ex_str = ex.to_string();
                        let ex_str2 = ex_str.clone();
                        let ex_label = ex_str.clone();
                        view! {
                            <button
                                class=move || {
                                    if selected_lift.get() == ex_str {
                                        "btn btn-selected".to_string()
                                    } else {
                                        "btn".to_string()
                                    }
                                }
                                on:click={
                                    let ex_str2 = ex_str2.clone();
                                    move |_| set_selected_lift.set(ex_str2.clone())
                                }
                            >{ex_label}</button>
                        }
                    }).collect_view()}
                </div>

                <Suspense fallback=|| view! { <div class="loading">"Loading…"</div> }>
                    {move || lift_history.get().map(|result| match result {
                        Err(e) => view! {
                            <div class="alert alert-error">"Error: " {e.to_string()}</div>
                        }.into_any(),
                        Ok(points) => view! {
                            <LineChart
                                title=move || format!("{} — working weight (kg)", selected_lift.get())
                                points=points.clone()
                                y_unit="kg".to_string()
                            />
                        }.into_any(),
                    })}
                </Suspense>
            </div>

            // ── Bodyweight chart ───────────────────────────────────────────
            <div class="chart-section">
                <h2>"Bodyweight"</h2>
                <Suspense fallback=|| view! { <div class="loading">"Loading…"</div> }>
                    {move || bw_history.get().map(|result| match result {
                        Err(e) => view! {
                            <div class="alert alert-error">"Error: " {e.to_string()}</div>
                        }.into_any(),
                        Ok(raw) => {
                            let points: Vec<LiftHistoryPoint> = raw
                                .into_iter()
                                .map(|(date, weight)| LiftHistoryPoint { date, weight })
                                .collect();
                            view! {
                                <LineChart
                                    title=move || "Bodyweight (kg)".to_string()
                                    points=points.clone()
                                    y_unit="kg".to_string()
                                />
                            }.into_any()
                        }
                    })}
                </Suspense>
            </div>
        </div>
    }
}

// ── Generic SVG line chart ─────────────────────────────────────────────────────

const SVG_W: f64 = 800.0;
const SVG_H: f64 = 300.0;
const PAD_L: f64 = 55.0;
const PAD_R: f64 = 20.0;
const PAD_T: f64 = 20.0;
const PAD_B: f64 = 40.0;

#[component]
pub fn LineChart(
    #[prop(into)] title:  Signal<String>,
    points: Vec<LiftHistoryPoint>,
    y_unit: String,
) -> impl IntoView {
    if points.is_empty() {
        return view! {
            <div class="chart-empty">
                <p>"No data yet for " {move || title.get()}</p>
            </div>
        }
        .into_any();
    }

    let min_y = points.iter().map(|p| p.weight).fold(f64::INFINITY, f64::min);
    let max_y = points.iter().map(|p| p.weight).fold(f64::NEG_INFINITY, f64::max);
    // Add 10 % padding around the y range so points aren't at the very edge.
    let range_y = (max_y - min_y).max(1.0);
    let lo_y = (min_y - range_y * 0.1).max(0.0);
    let hi_y = max_y + range_y * 0.1;

    let chart_w = SVG_W - PAD_L - PAD_R;
    let chart_h = SVG_H - PAD_T - PAD_B;

    let n = points.len();

    // Map (i, weight) → (svg_x, svg_y)
    let to_x = |i: usize| PAD_L + (i as f64 / (n - 1).max(1) as f64) * chart_w;
    let to_y = |w: f64| PAD_T + chart_h - (w - lo_y) / (hi_y - lo_y) * chart_h;

    // Build SVG polyline points string
    let polyline_pts: String = points
        .iter()
        .enumerate()
        .map(|(i, p)| format!("{:.1},{:.1}", to_x(i), to_y(p.weight)))
        .collect::<Vec<_>>()
        .join(" ");

    // Circle nodes
    let circles: Vec<_> = points
        .iter()
        .enumerate()
        .map(|(i, p)| {
            let cx = format!("{:.1}", to_x(i));
            let cy = format!("{:.1}", to_y(p.weight));
            let label = format!("{:.1} {}", p.weight, y_unit);
            let date_label = p.date.get(..10).unwrap_or(&p.date).to_string();
            view! {
                <circle cx=cx.clone() cy=cy.clone() r="5" class="chart-dot">
                    <title>{format!("{}: {}", date_label, label)}</title>
                </circle>
            }
        })
        .collect();

    // Y-axis tick labels (5 ticks)
    let y_ticks: Vec<_> = (0..=4)
        .map(|t| {
            let frac = t as f64 / 4.0;
            let w    = lo_y + frac * (hi_y - lo_y);
            let y    = to_y(w);
            view! {
                <g>
                    <line
                        x1=format!("{:.1}", PAD_L)
                        y1=format!("{:.1}", y)
                        x2=format!("{:.1}", SVG_W - PAD_R)
                        y2=format!("{:.1}", y)
                        class="chart-grid"
                    />
                    <text
                        x=format!("{:.1}", PAD_L - 6.0)
                        y=format!("{:.1}", y + 4.0)
                        class="chart-label"
                        text-anchor="end"
                    >{format!("{:.0}", w)}</text>
                </g>
            }
        })
        .collect();

    // X-axis date labels (show first, last, and up to 3 middle)
    let x_label_indices: Vec<usize> = if n <= 5 {
        (0..n).collect()
    } else {
        vec![0, n / 4, n / 2, 3 * n / 4, n - 1]
    };
    let x_labels: Vec<_> = x_label_indices
        .into_iter()
        .map(|i| {
            let x   = to_x(i);
            let lbl = points[i].date.get(..10).unwrap_or(&points[i].date).to_string();
            view! {
                <text
                    x=format!("{:.1}", x)
                    y=format!("{:.1}", SVG_H - 6.0)
                    class="chart-label"
                    text-anchor="middle"
                >{lbl}</text>
            }
        })
        .collect();

    let title_str = title.get_untracked();
    let svg_w_str = SVG_W.to_string();
    let svg_h_str = SVG_H.to_string();

    view! {
        <div class="chart-wrapper">
            <p class="chart-title">{title_str}</p>
            <svg
                viewBox=format!("0 0 {} {}", svg_w_str, svg_h_str)
                class="chart-svg"
                role="img"
                aria-label=move || title.get()
            >
                // Grid lines + y labels
                {y_ticks}
                // X labels
                {x_labels}
                // The line itself
                <polyline points=polyline_pts class="chart-line" fill="none"/>
                // Data point circles
                {circles}
                // Axes
                <line
                    x1=format!("{:.1}", PAD_L)
                    y1=format!("{:.1}", PAD_T)
                    x2=format!("{:.1}", PAD_L)
                    y2=format!("{:.1}", SVG_H - PAD_B)
                    class="chart-axis"
                />
                <line
                    x1=format!("{:.1}", PAD_L)
                    y1=format!("{:.1}", SVG_H - PAD_B)
                    x2=format!("{:.1}", SVG_W - PAD_R)
                    y2=format!("{:.1}", SVG_H - PAD_B)
                    class="chart-axis"
                />
            </svg>
        </div>
    }
    .into_any()
}
