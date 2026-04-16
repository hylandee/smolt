pub mod app;
pub mod components;
pub mod models;
pub mod server_fns;

// ── WASM entry point ──────────────────────────────────────────────────────────
#[cfg(feature = "hydrate")]
#[wasm_bindgen::prelude::wasm_bindgen(start)]
pub fn hydrate() {
    use app::App;
    console_error_panic_hook::set_once();
    leptos::mount::hydrate_body(App);
}
