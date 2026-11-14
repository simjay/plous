use rune::{Cx, El, TextStyle, Transition, col, row};

use super::{Icon, LABEL, Paint, State, symbol, txt};

#[cfg(target_arch = "wasm32")]
#[wasm_bindgen::prelude::wasm_bindgen]
extern "C" {
    #[wasm_bindgen(js_namespace = window, js_name = plousExampleStep)]
    fn browser_step() -> usize;
}

pub(super) fn initial_step() -> usize {
    #[cfg(target_arch = "wasm32")]
    { browser_step() }
    #[cfg(not(target_arch = "wasm32"))]
    { 0 }
}

const MONO: TextStyle = TextStyle::new(13.0, 500.0).lh(20.0).mono();
const COPY: TextStyle = TextStyle::new(14.0, 400.0).lh(22.0);
const TITLE: TextStyle = TextStyle::new(16.0, 600.0).lh(24.0);

fn panel(key: &str, title: &str, p: Paint) -> El {
    col().key(key).role("region").label(title).w_full().gap(12.0)
        .border_sides([0.0, 0.0, 1.0, 0.0], p.line).pb(12.0)
}

fn controls(s: &State, cx: &Cx<State>, labels: &[&str], p: Paint) -> El {
    let reduced = cx.system_reduced_motion();
    row().key("example-controls").w_full()
        .children(labels.iter().enumerate().map(|(step, &label)| {
            let selected = s.example == step;
            let control = row().key(format!("example-step-{step}")).role("button")
                .expanded(selected).focusable(true).focus_ring(2.0, 2.0, p.blue)
                .w(0.0).grow(1.0).min_h(44.0).px(4.0).center()
                .border_sides([0.0, 0.0, 2.0, 0.0], if selected {p.blue} else {p.line})
                .fill(p.bg).hover(p.surface)
                .child(txt(&format!("example-control-label-{step}"), label, LABEL, p.ink))
                .on_press_tx(move |s: &mut State, tx| {
                    s.example = step;
                    if reduced { tx.instant(); }
                });
            #[cfg(target_arch = "wasm32")]
            let control = control.label(label);
            #[cfg(not(target_arch = "wasm32"))]
            let control = control.label(format!("example-step-{step}"));
            control
        }))
}

fn frames(s: &State, frames: impl IntoIterator<Item = El>) -> El {
    let output = col().key("example-frames").role("status").w_full().instant_changes();
    #[cfg(not(target_arch = "wasm32"))]
    let output = output.label("example-frames");
    output
        .children(frames.into_iter().enumerate().filter_map(|(step, frame)| {
            #[cfg(not(target_arch = "wasm32"))]
            let visible = { let _ = s; true };
            #[cfg(target_arch = "wasm32")]
            let visible = step == s.example;
            visible.then(|| {
                let frame = frame.key(format!("example-frame-{step}")).w_full().transition(Transition::opacity());
                // The static exporter keeps labels on divs, but only emits IDs for semantic tags.
                #[cfg(not(target_arch = "wasm32"))]
                let frame = frame.label(format!("example-frame-{step}"));
                frame
            })
        }))
}

pub(super) fn panels(s: &State, cx: &Cx<State>, p: Paint) -> El {
    let names = ["Sidebar", "Main", "Assistant"];
    let descriptions = [
        "Choose a workspace or resource type.",
        "Browse resources and open details.",
        "Chat with the selected agent.",
    ];
    let views = descriptions.into_iter().map(|description| {
        col().child(txt("panel-description", description, COPY, p.ink).w_full())
    });
    panel("panel-example", "Panel focus diagram", p)
        .child(controls(s, cx, &names, p))
        .child(frames(s, views))
}

pub(super) fn target(s: &State, cx: &Cx<State>, p: Paint) -> El {
    let path = row().w_full().center_y().gap(4.0).flex_wrap();
    let path = ["Coding agent", "Plous MCP", "Kubernetes context"].iter().enumerate()
        .fold(path, |path, (i, name)| {
            let path = if i == 0 {path} else {path.child(symbol(Icon::Arrow, 16.0, p.muted))};
            path.child(row().px(8.0).py(4.0).radius(4.0).fill(p.bg)
                .child(txt(&format!("target-step-{i}"), *name, MONO, p.ink)))
        });
    let request = col().gap(8.0).child(path)
        .child(txt("target-help", "The coding agent calls Plous MCP, which reads from the Kubernetes context selected for that request.", COPY, p.muted).w_full());
    let example = col().gap(8.0)
        .child(txt("target-tool", "kubernetes_context_read", MONO, p.ink))
        .child(txt("target-values", "Context: minikube    Default namespace: shop", MONO, p.ink).w_full())
        .child(txt("target-note", "Check both values before asking your coding agent to inspect workloads.", COPY, p.muted).w_full());
    panel("target-example", "Where an MCP request goes", p)
        .child(txt("target-example-title", "Where an MCP request goes", TITLE, p.ink))
        .child(controls(s, cx, &["Request path", "Example target"], p))
        .child(frames(s, [request, example]))
}
