use rune::{Cx, El, Span, TextStyle, Transition, col, row};

use super::{Paint, State, symbol, txt};

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
    col().key(key).role("region").label(title).w_full().gap(16.0).padding(16.0)
        .fill(p.surface).border(1.0, p.line).radius(12.0)
        .child(txt(&format!("{key}-title"), title, TITLE, p.ink))
}

fn controls(s: &State, cx: &Cx<State>, labels: &[&str], p: Paint) -> El {
    let reduced = cx.system_reduced_motion();
    row().key("example-controls").w_full().gap(4.0).flex_wrap()
        .children(labels.iter().enumerate().map(|(step, &label)| {
            let selected = s.example == step;
            let control = row().key(format!("example-step-{step}")).role("button")
                .expanded(selected).focusable(true).focus_ring(2.0, 2.0, p.blue)
                .min_h(40.0).px(8.0).center_y().radius(6.0).border(1.0, if selected {p.blue} else {p.line})
                .fill(if selected {p.wash} else {p.bg}).hover(p.wash)
                .child(txt(&format!("example-control-label-{step}"), label, MONO, p.ink))
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
        "Sidebar has focus. Select a workspace or resource type.",
        "Main has focus. Browse resources or open a detail view.",
        "Assistant has focus. Work with the selected agent.",
    ];
    let views = descriptions.iter().enumerate().map(|(active, description)| {
        let strip = row().w_full().gap(4.0).children(names.iter().enumerate().map(|(i, name)| {
            col().w(0.0).grow(1.0).min_h(48.0).px(4.0).center()
                .border(1.0, if i == active {p.blue} else {p.line})
                .radius(6.0).fill(if i == active {p.wash} else {p.bg})
                .child(txt(&format!("panel-name-{i}"), *name, MONO, p.ink))
        })).a11y_hidden();
        col().gap(8.0).child(strip)
            .child(txt("panel-description", *description, COPY, p.ink).w_full())
    });
    let help = "In Plous, Alt+Right moves to the next visible panel. Alt+Left moves back.";
    let keycaps = ["Alt+Right", "Alt+Left"].into_iter().filter_map(|key| {
        help.find(key).map(|at| Span::new(at, at + key.len()).mono().color(p.ink).bg(p.bg, 2.0))
    }).collect::<Vec<_>>();
    panel("panel-example", "Panel focus diagram", p)
        .child(txt("panel-help", help, COPY, p.muted).spans(keycaps).w_full())
        .child(controls(s, cx, &names, p))
        .child(frames(s, views))
}

pub(super) fn target(s: &State, cx: &Cx<State>, p: Paint) -> El {
    let path = row().w_full().center_y().gap(4.0).flex_wrap();
    let path = ["Coding agent", "Plous MCP", "Kubernetes context"].iter().enumerate()
        .fold(path, |path, (i, name)| {
            let path = if i == 0 {path} else {path.child(symbol(3, 14.0, p.muted))};
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
        .child(controls(s, cx, &["Request path", "Example target"], p))
        .child(frames(s, [request, example]))
}
