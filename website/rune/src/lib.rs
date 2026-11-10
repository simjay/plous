use std::collections::BTreeSet;
use std::sync::OnceLock;
use rune::{AppSpec, Color, Cx, El, ImageCache, SurfaceFit, TextStyle, col, el, row, text};
use rune_kit::{Markdown, MarkdownStyle};
use rune_kit::syntax::Syntax;
use serde::Deserialize;

#[derive(Deserialize)]
struct Link { title: String, href: String, #[serde(default)] description: String }
#[derive(Deserialize)]
struct Group { label: String, items: Vec<Link> }
#[derive(Deserialize)]
struct Heading {
    title: String, id: String, level: u8,
    #[cfg(target_arch="wasm32")] line: usize,
    #[cfg(not(target_arch="wasm32"))] offset: usize,
}
#[derive(Deserialize)]
struct Page { route: String, href: String, title: String, body: String, headings: Vec<Heading>, section: String, developers: bool, edit: String }
#[derive(Deserialize)]
struct Content { version: String, base: String, root: String, navigation: Vec<Group>, paths: Vec<Link>, pages: Vec<Page> }
fn content() -> &'static Content {
    static CONTENT: OnceLock<Content> = OnceLock::new();
    CONTENT.get_or_init(|| serde_json::from_str(include_str!("../.generated/content.json")).expect("prepared documentation"))
}

#[derive(Clone, Copy)]
struct Paint { bg: Color, surface: Color, ink: Color, muted: Color, line: Color, blue: Color, wash: Color }
fn hex(value: &str) -> Color { rune::color::hex(value).expect("site palette") }
impl Paint {
    fn new(dark: bool) -> Self {
        if dark { Self { bg:hex("#101722"),surface:hex("#182232"),ink:hex("#e5edf8"),muted:hex("#a7b5c9"),line:hex("#2b3a50"),blue:hex("#8db4ff"),wash:hex("#203454") } }
        else { Self { bg:hex("#ffffff"),surface:hex("#f6f8fc"),ink:hex("#172033"),muted:hex("#56657a"),line:hex("#e2e8f0"),blue:hex("#326ce5"),wash:hex("#edf3ff") } }
    }
}
const BODY: TextStyle = TextStyle::new(16.0,400.0).lh(27.0);
const LABEL: TextStyle = TextStyle::new(14.0,500.0).lh(21.0);
const SMALL: TextStyle = TextStyle::new(13.0,400.0).lh(20.0);
// Shape paths export as SVG and use the same geometry in the GPU view.
fn symbol(kind: usize,size: f32,color: Color) -> El {
    use rune::{Path,Shape,LineCap,canvas};
    let path=|points: &[[f32;2]]| {
        let mut value=Path::new().move_to(points[0]);
        for point in &points[1..] {value=value.line_to(*point);}
        value
    };
    let paths=match kind {
        0=>vec![path(&[[5.0,7.0],[10.0,12.0],[5.0,17.0]]),path(&[[13.0,17.0],[19.0,17.0]])],
        1=>vec![path(&[[9.0,7.0],[4.0,12.0],[9.0,17.0]]),path(&[[15.0,7.0],[20.0,12.0],[15.0,17.0]]),path(&[[13.0,4.0],[11.0,20.0]])],
        2=>vec![path(&[[12.0,3.0],[14.5,9.5],[21.0,12.0],[14.5,14.5],[12.0,21.0],[9.5,14.5],[3.0,12.0],[9.5,9.5],[12.0,3.0]])],
        _=>vec![path(&[[5.0,12.0],[19.0,12.0]]),path(&[[13.0,6.0],[19.0,12.0],[13.0,18.0]])],
    };
    let mut shape=Shape::new().view_box(24.0,24.0);
    for path in paths {shape=shape.stroke_path(&path,1.7,LineCap::Round,color);}
    canvas(shape).size(size,size).a11y_hidden()
}

struct State { page: Option<usize>, dark: bool, open: BTreeSet<usize>, images: Option<ImageCache> }
#[cfg(target_arch="wasm32")]
#[wasm_bindgen::prelude::wasm_bindgen]
extern "C" {
    #[wasm_bindgen(js_namespace=window,js_name=plousRunePage)] fn browser_page() -> String;
    #[wasm_bindgen(js_namespace=window,js_name=plousRuneDark)] fn browser_dark() -> bool;
    #[wasm_bindgen(js_namespace=window,js_name=plousSetDark)] fn set_browser_dark(dark: bool);
    #[wasm_bindgen(js_namespace=window,js_name=plousOpenSearch)] fn open_search();
    #[wasm_bindgen(js_namespace=window,js_name=plousOpenMenu)] fn open_menu();
    #[wasm_bindgen(js_namespace=window,js_name=plousOpenVersions)] fn open_versions();
    #[wasm_bindgen(js_namespace=window,js_name=plousRuneMarkdown)] fn browser_markdown(source: &str) -> String;
}
impl State {
    fn new() -> Self {
        #[cfg(target_arch="wasm32")]
        let route = browser_page();
        #[cfg(not(target_arch="wasm32"))]
        let route = std::env::var("PLOUS_SITE_PAGE").unwrap_or_default();
        let page = content().pages.iter().position(|p| p.route==route);
        #[cfg(target_arch="wasm32")]
        let dark = browser_dark();
        #[cfg(not(target_arch="wasm32"))]
        let dark = false;
        let mut open = BTreeSet::new();
        for (i,g) in content().navigation.iter().enumerate() {
            #[cfg(target_arch="wasm32")]
            let selected = i==0 || page.is_some_and(|p| g.label==content().pages[p].section);
            #[cfg(not(target_arch="wasm32"))]
            let selected = { let _=g; true };
            if selected { open.insert(i); }
        }
        Self { page,dark,open,images:None }
    }
}
fn txt(key: &str,value: impl Into<String>,font: TextStyle,color: Color) -> El { text(value).key(key).font(font).color(color).wrap().selectable(true) }
fn rule(key: &str,p: Paint) -> El { el().key(key).w_full().h(1.0).fill(p.line).a11y_hidden() }
fn link(key: &str,label: &str,url: &str,p: Paint) -> El {
    row().key(key).href(url).py(5.0).hover_group().child(txt(&format!("{key}-text"),label,LABEL,p.muted).group_color(p.blue))
}
fn button(key: &str,label: &str,p: Paint) -> El {
    row().key(key).role("button").label(label).focusable(true).h(36.0).px(12.0).center_y().fill(p.surface).radius(6.0).hover(p.wash).press_scale(0.98)
        .child(txt(&format!("{key}-text"),label,LABEL,p.ink))
}
fn asset(name: &str) -> String {
    #[cfg(target_arch="wasm32")]
    return format!("{}assets/{name}",content().base);
    #[cfg(not(target_arch="wasm32"))]
    format!("assets/{name}")
}
fn picture(s: &State,cx: &Cx<State>,key: &str,name: &str,width: f32,alt: &str) -> El {
    let out=el().key(key).role("image").label(alt);
    if let Some(cache)=&s.images {
        #[cfg(target_arch="wasm32")]
        let image=cache.url(&asset(name),width,cx.env().scale);
        #[cfg(not(target_arch="wasm32"))]
        let image=cache.file(&asset(name),width,cx.env().scale);
        out.pixels_fit(image.pixels(),SurfaceFit::Fit)
    } else { out }
}
fn header(s: &State,cx: &Cx<State>,phone: bool,p: Paint) -> El {
    let brand=row().key("brand").href(&content().root).label("Plous home").gap(10.0).center_y()
        .child(picture(s,cx,"brand-wordmark",if s.dark {"lockup-dark.svg"} else {"lockup.svg"},104.0,"Plous").size(104.0,104.0*124.0/375.0));
    let mut nav=row().key("primary").role("navigation").label("Primary").center_y().gap(if phone {6.0} else {24.0});
    let docs=link("nav-docs","Documentation",&format!("{}docs/",content().base),p);
    let developers=link("nav-developers","Developers",&format!("{}docs/contributing/",content().base),p);
    nav=nav.child(if phone {docs.w(0.0).h(0.0).clip_rounded(0.0).a11y_hidden()} else {docs})
        .child(if phone {developers.w(0.0).h(0.0).clip_rounded(0.0).a11y_hidden()} else {developers});
    nav=nav.child(button("search","Search",p).px(if phone {8.0} else {12.0}).label("Search documentation").on_press(|_: &mut State| { #[cfg(target_arch="wasm32")] open_search(); }));
    nav=nav.child(button("theme",if s.dark {"Light"} else {"Dark"},p).px(if phone {8.0} else {12.0}).label("Change color theme").on_press(|s: &mut State| {
        s.dark=!s.dark;
        #[cfg(target_arch="wasm32")] set_browser_dark(s.dark);
    }));
    let menu=button("menu","Menu",p).px(8.0).label("Open navigation").on_press(|_: &mut State| { #[cfg(target_arch="wasm32")] open_menu(); });
    nav=nav.child(if phone {menu} else {menu.w(0.0).h(0.0).px(0.0).clip_rounded(0.0).a11y_hidden()});
    row().key("header").role("banner").w_full().h(76.0).justify_between().center_y().child(brand).child(nav)
}
fn workflows(p: Paint,phone: bool) -> El {
    let mut cards=(if phone {col()} else {row().items_stretch()}).key("workflows").w_full().gap(16.0);
    for (i,path) in content().paths.iter().enumerate() {
        let card=col().key(format!("workflow-{i}")).href(&path.href).padding(24.0).gap(16.0).fill(p.bg).border(1.0,p.line).radius(10.0).hover_group()
            .child(symbol(i,24.0,p.blue))
            .child(row().w_full().center_y().gap(12.0)
                .child(txt(&format!("workflow-title-{i}"),&path.title,TextStyle::new(19.0,600.0).lh(27.0),p.ink).w(0.0).grow(1.0).group_color(p.blue))
                .child(symbol(3,18.0,p.blue)))
            .child(txt(&format!("workflow-description-{i}"),&path.description,BODY,p.muted));
        cards=cards.child(if phone {card.w_full()} else {card.w(0.0).grow(1.0)});
    }
    cards
}
fn home(s: &State,cx: &Cx<State>,phone: bool,p: Paint) -> El {
    let size=if phone {40.0} else {64.0};
    let intro=col().w_full().gap(22.0).pt(if phone {40.0} else {72.0}).pb(44.0)
        .child(txt("top","A clearer view of\nyour Kubernetes.",TextStyle::new(size,600.0).lh(size+8.0).tracking(-1.2),p.ink).role("heading").level(1))
        .child(version_button(p))
        .child(txt("home-intro","Inspect clusters, follow workload evidence, and investigate with your AI assistant. Choose the workflow that fits how you work.",TextStyle::new(19.0,400.0).lh(31.0),p.muted).max_w(650.0));
    let product=col().key("product").w_full().gap(16.0).py(48.0)
        .child(row().href(format!("{}assets/plous-overview.png",content().base)).label("Open full size Plous workspace screenshot").w_full()
            .child(picture(s,cx,"product-image","plous-overview.png",1200.0,"Plous workspace overview with cluster health and the Codex assistant.").w_full().aspect_ratio(3024.0/1964.0).radius(10.0).border(1.0,p.line)))
        .child(row().w_full().justify_between().center_y().child(txt("product-caption","The Plous workspace",SMALL,p.muted)).child(link("product-full","View full size",&format!("{}assets/plous-overview.png",content().base),p)));
    let mut integrations=(if phone {col()} else {row().justify_between().center_y()}).w_full().gap(20.0).py(28.0).child(txt("integrations-label","Explore the integrations",LABEL,p.muted));
    for (name,path) in [("Kubernetes","kubernetes"),("Helm","helm"),("Argo CD","argo"),("Flux","flux")] {
        integrations=integrations.child(link(&format!("integration-{path}"),name,&format!("{}docs/integrations/{path}/",content().base),p));
    }
    let contributor=col().w_full().gap(16.0).py(48.0)
        .child(txt("contribute-title","Built in the open",TextStyle::new(28.0,600.0).lh(36.0),p.ink).role("heading").level(2))
        .child(txt("contribute-copy","Plous is in early development. Read the guides, report a bug, or help improve the project.",BODY,p.muted).max_w(640.0))
        .child(link("contributor-guide","Read the contributor guide",&format!("{}docs/contributing/",content().base),p));
    col().w_full().child(intro).child(workflows(p,phone)).child(product).child(rule("integrations-rule",p)).child(integrations).child(rule("contribute-rule",p)).child(contributor)
}
fn version_button(p: Paint) -> El {
    button("versions",&format!("{} · Versions",content().version),p).label("Choose documentation version").on_press(|_: &mut State| {
        #[cfg(target_arch="wasm32")] open_versions();
    })
}
fn sidebar(s: &State,page: &Page,phone: bool,p: Paint) -> El {
    let mut nav=col().key("sidebar").role("navigation").label("Documentation navigation").w(236.0).gap(6.0).pr(20.0).py(28.0);
    nav=nav.child(version_button(p));
    nav=nav.child(link("sidebar-root",if page.developers {"User documentation"} else {"Documentation"},&format!("{}docs/",content().base),p));
    for (i,group) in content().navigation.iter().enumerate().filter(|(_,g)| (g.label=="Developers")==page.developers) {
        let opened=s.open.contains(&i);
        let title=button(&format!("group-toggle-{i}"),&group.label,p).w_full().fill(p.bg).mt(10.0)
            .on_press(move |s: &mut State| { if !s.open.remove(&i) {s.open.insert(i);} });
        let mut items=col().key(format!("group-items-{i}")).w_full().gap(2.0).pt(4.0);
        for (j,item) in group.items.iter().enumerate() {
            let selected=item.href.split('#').next()==Some(page.href.as_str()) && !item.href.contains('#');
            items=items.child(row().key(format!("nav-link-{i}-{j}")).href(&item.href).w_full().py(7.0).px(12.0).radius(5.0).fill(if selected {p.wash} else {p.bg}).hover(p.wash)
                .child(txt(&format!("nav-label-{i}-{j}"),&item.title,TextStyle::new(14.0,if selected {600.0} else {400.0}).lh(21.0),if selected {p.blue} else {p.muted})));
        }
        if !opened {items=items.h(0.0).clip_rounded(0.0).a11y_hidden();}
        nav=nav.child(title).child(items);
    }
    if phone {nav.w(0.0).h(0.0).pr(0.0).py(0.0).clip_rounded(0.0).a11y_hidden()} else {nav}
}
fn markdown(s: &State,page: &Page,width: f32,p: Paint) -> El {
    #[cfg(target_arch="wasm32")]
    let source=browser_markdown(&page.body);
    #[cfg(not(target_arch="wasm32"))]
    let source=page.body.clone();
    let mut style=MarkdownStyle::neutral(s.dark).text(p.ink).muted(p.muted).link(p.blue).rule(p.line)
        .body_font(BODY).heading_font(2,TextStyle::new(25.0,600.0).lh(34.0).tracking(-0.3))
        .heading_font(3,TextStyle::new(19.0,600.0).lh(28.0))
        .inline_code_bg(p.surface).code_block(p.surface,p.line).shell_code(p.surface,p.line,p.ink)
        .code_block_font(TextStyle::new(13.0,400.0).lh(22.0).mono()).code_block_padding(18.0,16.0)
        .table_font(TextStyle::new(14.0,400.0).lh(23.0)).table_cell_padding(12.0,10.0)
        .table_full_width(true).table_header_fill(p.surface).table_rules(p.line).text_width(width).wide_fade(0.0);
    let green=hex(if s.dark {"#94d7af"} else {"#206a43"});
    let violet=hex(if s.dark {"#cfadff"} else {"#7045ba"});
    let amber=hex(if s.dark {"#e7bc83"} else {"#905500"});
    let red=hex(if s.dark {"#f3a5af"} else {"#b63e50"});
    for (kind,color) in [(Syntax::Keyword,violet),(Syntax::String,green),(Syntax::Number,amber),(Syntax::Comment,p.muted),(Syntax::Type,violet),(Syntax::Function,p.blue),(Syntax::Property,p.blue),(Syntax::Punctuation,p.muted),(Syntax::Inserted,green),(Syntax::Deleted,red),(Syntax::Parameter,p.ink),(Syntax::Heading,violet),(Syntax::Link,p.blue),(Syntax::Operator,p.muted),(Syntax::Regex,green),(Syntax::Attribute,amber),(Syntax::Variable,p.ink)] {
        style=style.syntax(kind,color);
    }
    let md=Markdown::new(source.clone()).selectable(true).scroll_wide(true)
        .block_keys(page.headings.iter().map(|h| {
            #[cfg(target_arch="wasm32")]
            let offset=source.split_inclusive('\n').take(h.line).map(str::len).sum();
            #[cfg(not(target_arch="wasm32"))]
            let offset=h.offset;
            (offset,h.id.clone())
        }))
        .on_link_tx(|_: &mut State,url,tx| {let _=tx.open_url_same_tab(url);})
        .code_frame(move |card,code| {
            let value=code.source.to_owned();
            col().w_full().gap(8.0).child(row().w_full().justify_between().center_y()
                .child(txt(&format!("language-{}",code.index),code.language.unwrap_or("text"),SMALL,p.muted))
                .child(button(&format!("copy-{}",code.index),"Copy",p).label(format!("Copy code block {}",code.index+1)).on_press_tx(move |_: &mut State,tx| tx.copy(value.clone()))))
                .child(card)
        });
    md.render(&style)
}
fn article(s: &State,cx: &Cx<State>,page: &Page,phone: bool,p: Paint) -> El {
    let width=cx.layout_size("article").map_or(if phone {350.0} else {700.0},|v|v[0]);
    let mut body=col().key("article").w_full().max_w(760.0).gap(24.0).py(if phone {28.0} else {44.0})
        .child(txt("section-name",&page.section,LABEL,p.blue))
        .child(txt("top",&page.title,TextStyle::new(if phone {32.0} else {38.0},600.0).lh(if phone {40.0} else {48.0}).tracking(-0.6),p.ink).role("heading").level(1));
    if page.route=="docs/" { body=body.child(txt("index-intro","Choose how you want to work with Plous.",BODY,p.muted)).child(workflows(p,true)); }
    body.child(markdown(s,page,width,p)).child(rule("edit-rule",p)).child(link("edit-source",if content().version=="Development" {"Edit this page on GitHub"} else {"View tagged source"},&page.edit,p))
}
fn toc(page: &Page,phone: bool,p: Paint) -> El {
    let mut out=col().key("contents").role("navigation").label("On this page").w(180.0).gap(8.0).py(44.0)
        .child(txt("toc-title","On this page",LABEL,p.ink).mb(8.0));
    for (i,h) in page.headings.iter().enumerate().filter(|(_,h)|h.level==2) {
        out=out.child(link(&format!("toc-{i}"),&h.title,&format!("#{}",h.id),p));
    }
    if phone {out.w_full().pt(0.0)} else {out}
}
fn view(s: &State,cx: &mut Cx<State>) -> El {
    let measured=cx.layout_size("page").map_or(1440.0,|v|v[0]);
    let phone=measured<1000.0;
    let narrow=measured<700.0;
    let p=Paint::new(s.dark);
    let gutter=if narrow {20.0} else {32.0};
    let frame=|e: El| e.w_full().max_w(1480.0).px(gutter);
    let main=if let Some(index)=s.page {
        let page=&content().pages[index];
        let columns=(if phone {col()} else {row().items_start()}).w_full().gap(if phone {0.0} else {40.0});
        let document=article(s,cx,page,narrow,p);
        let content_columns=if measured<1280.0 {
            col().w_full().gap(0.0).child(document.w_full()).child(toc(page,true,p))
        } else {
            row().items_start().w(0.0).grow(1.0).gap(40.0).child(document.w(0.0).grow(1.0)).child(toc(page,false,p))
        };
        let content_columns=if phone {content_columns.w_full()} else {content_columns.w(0.0).grow(1.0)};
        columns.child(sidebar(s,page,phone,p)).child(content_columns)
    } else {home(s,cx,narrow,p)};
    let footer=col().w_full().gap(18.0).pt(24.0).pb(32.0).child(rule("footer-rule",p))
        .child((if narrow {col().gap(12.0)} else {row().justify_between().center_y()})
            .child(txt("footer-note","Plous · Open source · Early development",SMALL,p.muted))
            .child(row().gap(24.0).child(link("footer-developers","Developers",&format!("{}docs/contributing/",content().base),p)).child(link("footer-github","GitHub","https://github.com/simjay/plous",p))));
    col().key("page").w_full().fill(p.bg).items_center()
        .child(frame(header(s,cx,phone,p))).child(frame(rule("header-rule",p)))
        .child(frame(main).key("main").role("main")).child(frame(footer))
}
#[cfg_attr(target_arch="wasm32",wasm_bindgen::prelude::wasm_bindgen(start))]
pub fn start() {
    let initial=State::new();
    let title=initial.page.map_or("Plous documentation",|i|content().pages[i].title.as_str());
    let spec=AppSpec { families:("Geist","Geist Mono"),setup:Some(|s: &mut State,w|s.images=Some(ImageCache::new(w))),..AppSpec::new(title,(1440.0,1000.0),State::new,view) };
    rune::App::new(spec).web_document().font_url(&asset("geist.ttf")).font_url(&asset("geist-mono.ttf")).run();
}
