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
struct Content {
    version: String,
    base: String,
    root: String,
    navigation: Vec<Group>,
    paths: Vec<Link>,
    #[serde(rename = "quickLinks")]
    quick_links: Vec<Link>,
    pages: Vec<Page>,
}
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
const SECTION: TextStyle = TextStyle::new(26.0,650.0).lh(38.0).tracking(-0.4);
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
        3=>vec![path(&[[5.0,12.0],[19.0,12.0]]),path(&[[13.0,6.0],[19.0,12.0],[13.0,18.0]])],
        4=>vec![path(&[[10.0,7.0],[15.0,12.0],[10.0,17.0]])],
        _=>vec![path(&[[7.0,10.0],[12.0,15.0],[17.0,10.0]])],
    };
    let mut shape=Shape::new().view_box(24.0,24.0);
    for path in paths {shape=shape.stroke_path(&path,1.7,LineCap::Round,color);}
    canvas(shape).size(size,size).a11y_hidden()
}

struct State { page: Option<usize>, dark: bool, open: BTreeSet<usize>, contents_open: bool, images: Option<ImageCache> }
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
        Self { page,dark,open,contents_open:false,images:None }
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
        let card=col().key(format!("workflow-{i}")).href(&path.href).padding(18.0).gap(12.0)
            .fill(p.surface).border(1.0,p.line).radius(10.0).hover(p.wash).hover_group()
            .child(row().justify_between().center_y()
                .child(row().size(36.0,36.0).center().fill(p.wash).radius(7.0).child(symbol(i,22.0,p.blue)))
                .child(symbol(3,18.0,p.blue)))
            .child(txt(&format!("workflow-title-{i}"),&path.title,TextStyle::new(18.0,600.0).lh(25.0),p.ink).w_full().group_color(p.blue))
            .child(txt(&format!("workflow-description-{i}"),&path.description,TextStyle::new(14.0,400.0).lh(22.0),p.muted).w_full());
        cards=cards.child(if phone {card.w_full()} else {card.w(0.0).grow(1.0)});
    }
    cards
}
fn index_links(p: Paint, narrow: bool) -> El {
    let links = content().quick_links.iter().enumerate().map(|(i, item)| {
        link(&format!("quick-link-{i}"), &item.title, &item.href, p)
            .w_full().py(11.0).border_sides([0.0, 0.0, 1.0, 0.0], p.line)
    });
    col().key("index-links").w_full().gap(8.0).mt(8.0)
        .child(txt("quick-links", "Quick links", SECTION, p.ink).role("heading").level(2))
        .child(col().key("quick-link-grid").w_full().equal_columns(if narrow { 1 } else { 2 })
            .gaps(0.0, 24.0).children(links))
}
fn home(s: &State,cx: &Cx<State>,phone: bool,p: Paint) -> El {
    let size=if phone {40.0} else {64.0};
    let intro=col().w_full().gap(22.0).pt(if phone {40.0} else {72.0}).pb(44.0)
        .child(txt("top","Kubernetes,\nin your terminal.",TextStyle::new(size,600.0).lh(size+8.0).tracking(-1.2),p.ink).role("heading").level(1))
        .child(version_button(p))
        .child(txt("home-intro","Browse resources, read logs, and debug failures from one terminal.",TextStyle::new(19.0,400.0).lh(31.0),p.muted).max_w(650.0));
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
        .child(txt("contribute-copy","Report a bug or contribute to Plous.",BODY,p.muted).max_w(640.0))
        .child(link("contributor-guide","Read the contributor guide",&format!("{}docs/contributing/",content().base),p));
    col().w_full().child(intro).child(workflows(p,phone)).child(product).child(rule("integrations-rule",p)).child(integrations).child(rule("contribute-rule",p)).child(contributor)
}
fn version_button(p: Paint) -> El {
    button("versions",&format!("{} · Versions",content().version),p).label("Choose documentation version").on_press(|_: &mut State| {
        #[cfg(target_arch="wasm32")] open_versions();
    })
}
fn sidebar(s: &State,page: &Page,phone: bool,p: Paint) -> El {
    let mut nav=col().key("sidebar").role("navigation").label("Documentation navigation")
        .w(228.0).gap(6.0).padding(14.0).mt(28.0).fill(p.surface).border(1.0,p.line).radius(10.0);
    nav=nav.child(version_button(p));
    nav=nav.child(link("sidebar-root",if page.developers {"User documentation"} else {"Documentation"},&format!("{}docs/",content().base),p));
    for (i,group) in content().navigation.iter().enumerate().filter(|(_,g)| (g.label=="Developers")==page.developers) {
        let opened=s.open.contains(&i);
        let title=button(&format!("group-toggle-{i}"),&group.label,p).w_full().mt(6.0)
            .justify_between().child(symbol(if opened {5} else {4}, 16.0, p.muted)).expanded(opened)
            .on_press(move |s: &mut State| { if !s.open.remove(&i) {s.open.insert(i);} });
        let mut items=col().key(format!("group-items-{i}")).w_full().gap(2.0).pt(4.0);
        for (j,item) in group.items.iter().enumerate() {
            let selected=item.href.split('#').next()==Some(page.href.as_str()) && !item.href.contains('#');
            items=items.child(row().key(format!("nav-link-{i}-{j}")).href(&item.href).w_full().py(7.0).px(12.0)
                .radius(5.0).fill(if selected {p.wash} else {p.surface}).hover(p.wash)
                .border_sides([0.0,0.0,0.0,if selected {3.0} else {0.0}],p.blue)
                .child(txt(&format!("nav-label-{i}-{j}"),&item.title,TextStyle::new(14.0,if selected {600.0} else {400.0}).lh(21.0),if selected {p.blue} else {p.muted}).w(0.0).grow(1.0)));
        }
        if !opened {items=items.h(0.0).clip_rounded(0.0).a11y_hidden();}
        nav=nav.child(title).child(items);
    }
    if phone {nav.w(0.0).h(0.0).padding(0.0).mt(0.0).clip_rounded(0.0).a11y_hidden()} else {nav}
}
fn markdown(s: &State,page: &Page,width: f32,p: Paint) -> El {
    #[cfg(target_arch="wasm32")]
    let source=browser_markdown(&page.body);
    #[cfg(not(target_arch="wasm32"))]
    let source=page.body.clone();
    let mut style=MarkdownStyle::neutral(s.dark).text(p.ink).muted(p.muted).link(p.blue).rule(p.line)
        .body_font(BODY).heading_font(2,SECTION)
        .heading_font(3,TextStyle::new(19.0,600.0).lh(30.0))
        .inline_code_bg(p.surface).code_block(Color::TRANSPARENT,Color::TRANSPARENT)
        .code_block_font(TextStyle::new(14.0,400.0).lh(24.0).mono()).code_block_padding(16.0,12.0)
        .table_font(TextStyle::new(14.0,400.0).lh(23.0)).table_cell_padding(12.0,10.0)
        .table_full_width(true).table_header_fill(p.surface).table_header_text(p.ink)
        .table_rules(p.line).text_width(width).wide_fade(0.0);
    if page.route == "docs/reference/keymap/" {
        style = style.inline_code_bg(p.wash).inline_code_pad(4.0).inline_code_font(0.92, 500.0)
            .table_cell_padding(12.0, 13.0).table_header_weight(600.0);
    }
    let code_blue=hex(if s.dark {"#8db4ff"} else {"#2458b8"});
    let green=hex(if s.dark {"#94d7af"} else {"#206a43"});
    let violet=hex(if s.dark {"#cfadff"} else {"#7045ba"});
    let amber=hex(if s.dark {"#e7bc83"} else {"#905500"});
    let red=hex(if s.dark {"#f3a5af"} else {"#b63e50"});
    for (kind,color) in [(Syntax::Keyword,violet),(Syntax::String,green),(Syntax::Number,amber),(Syntax::Comment,p.muted),(Syntax::Type,violet),(Syntax::Function,code_blue),(Syntax::Property,code_blue),(Syntax::Punctuation,p.muted),(Syntax::Inserted,green),(Syntax::Deleted,red),(Syntax::Parameter,code_blue),(Syntax::Heading,violet),(Syntax::Link,code_blue),(Syntax::Operator,p.muted),(Syntax::Regex,green),(Syntax::Attribute,amber),(Syntax::Variable,violet)] {
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
            let value = code.source.to_owned();
            let language = code.language.unwrap_or("text");
            let caption = match language {
                "sh" | "bash" | "zsh" | "shell" => "Shell command",
                "text" if code.language.is_some() => "Output",
                "text" => "Plain text",
                "toml" => "TOML",
                "json" => "JSON",
                "yaml" => "YAML",
                _ => language,
            };
            let fill = if caption == "Output" { p.bg } else { p.surface };
            col().role("region").label(format!("{language} code example")).w_full()
                .fill(fill).border(1.0,p.line).radius(8.0)
                .child(row().w_full().px(12.0).py(4.0).justify_between().center_y()
                    .border_sides([0.0,0.0,1.0,0.0],p.line)
                    .child(txt(&format!("language-{}",code.index),caption,TextStyle::new(12.0,500.0).lh(18.0).mono(),p.muted))
                    .child(button(&format!("copy-{}",code.index),"Copy",p).h(28.0).fill(fill)
                        .label(format!("Copy code block {}",code.index+1))
                        .on_press_tx(move |_: &mut State,tx| tx.copy(value.clone()))))
                .child(card)
        });
    md.render(&style)
}
fn article(s: &State,cx: &Cx<State>,page: &Page,narrow: bool,wide: bool,p: Paint) -> El {
    let width = cx.layout_size("article").map_or(if narrow {350.0} else {700.0}, |v| v[0]);
    let index = page.route == "docs/";
    let mut header = col().w_full().gap(8.0);
    if page.section != page.title {
        header = header.child(txt("section-name",&page.section,LABEL,p.muted));
    }
    header = header.child(txt("top",&page.title,TextStyle::new(if narrow {32.0} else {42.0},650.0)
        .lh(if narrow {40.0} else {50.0}).tracking(-0.9),p.ink).role("heading").level(1));
    let header = if !index && page.headings.iter().any(|heading| heading.level == 2) {
        header.child(inline_contents(s,page,wide,p))
    } else { header };
    let body = col().key("article").w_full().max_w(760.0).gap(24.0)
        .py(if narrow {28.0} else {42.0}).child(header);
    let body = if index {
        body.child(txt("index-intro","Choose your workflow",SECTION,p.ink).role("heading").level(2))
            .child(workflows(p,width < 660.0)).child(index_links(p,width < 540.0))
    } else {
        body.child(markdown(s,page,width,p))
    };
    body.child(rule("edit-rule",p)).child(link("edit-source",if content().version=="Development" {"Edit this page on GitHub"} else {"View source"},&page.edit,p))
}
fn contents_links(page: &Page, key: &str, p: Paint) -> El {
    col().key(key).role("navigation").label("On this page").w_full()
        .children(page.headings.iter().filter(|heading| heading.level == 2).map(|heading| {
            let id = format!("{key}-{}", heading.id);
            row().key(&id).href(format!("#{}",heading.id)).w_full().py(5.0).hover_group()
                .child(txt(&format!("{id}-text"),&heading.title,LABEL,p.muted).w(0.0).grow(1.0).group_color(p.blue))
        }))
}
fn inline_contents(s: &State, page: &Page, wide: bool, p: Paint) -> El {
    let toggle = button("contents-toggle","On this page",p).w_full().justify_between()
        .child(symbol(if s.contents_open {5} else {4}, 16.0, p.muted)).expanded(s.contents_open)
        .on_press(|s: &mut State| s.contents_open = !s.contents_open);
    let links = contents_links(page,"contents-inline-links",p).px(12.0).pb(8.0);
    #[cfg(target_arch="wasm32")]
    let links = if s.contents_open { links } else { links.h(0.0).padding(0.0).clip_rounded(0.0).a11y_hidden() };
    let panel = col().key("contents-inline").role("region").w_full().fill(p.surface).border(1.0,p.line).radius(8.0)
        .child(toggle).child(links);
    if wide { panel.h(0.0).clip_rounded(0.0).a11y_hidden() } else { panel }
}
fn toc(page: &Page,wide: bool,p: Paint) -> El {
    let panel = col().key("contents").role("region").w(172.0).mt(45.0).pl(16.0).gap(8.0)
        .border_sides([0.0,0.0,0.0,1.0],p.line)
        .child(txt("toc-title","On this page",LABEL,p.ink))
        .child(contents_links(page,"contents-desktop-links",p));
    if wide { panel } else { panel.w(0.0).h(0.0).mt(0.0).pl(0.0).clip_rounded(0.0).a11y_hidden() }
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
        let has_contents = page.route != "docs/" && page.headings.iter().any(|heading| heading.level == 2);
        let wide = measured >= 1280.0 && has_contents;
        let document = article(s,cx,page,narrow,wide,p);
        let content_columns = row().items_start().gap(if wide {32.0} else {0.0})
            .child(document.w(0.0).grow(1.0));
        let content_columns = if has_contents { content_columns.child(toc(page,wide,p)) } else { content_columns };
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
