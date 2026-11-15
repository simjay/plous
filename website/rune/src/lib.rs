use std::collections::{BTreeMap, BTreeSet};
use std::sync::OnceLock;
use rune::{AppSpec, Color, Cx, El, ImageCache, Span, SurfaceFit, TextStyle, col, el, row, text};
use rune_kit::{Markdown, MarkdownStyle};
use rune_kit::syntax::Syntax;
use serde::Deserialize;

mod examples;

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
#[serde(rename_all = "kebab-case")]
enum Example { PanelFocus, WorkspaceOverview, McpTarget }
impl Example {
    fn marker(&self) -> &'static str {
        match self {
            Self::PanelFocus => "plous-example:panel-focus",
            Self::WorkspaceOverview => "plous-example:workspace-overview",
            Self::McpTarget => "plous-example:mcp-target",
        }
    }
}
#[derive(Deserialize)]
struct Page {
    route: String, href: String, title: String, body: String, headings: Vec<Heading>,
    section: String, developers: bool, edit: String,
    #[serde(default)] examples: Vec<Example>,
}
#[derive(Deserialize)]
struct Content {
    version: String,
    base: String,
    root: String,
    navigation: Vec<Group>,
    #[serde(rename = "indexIntro")]
    index_intro: String,
    #[serde(rename = "installCommand")]
    install_command: String,
    paths: Vec<Link>,
    #[serde(rename = "quickLinks")]
    quick_links: Vec<Group>,
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
        if dark { Self { bg:hex("#14171d"),surface:hex("#1c212a"),ink:hex("#edf1f7"),muted:hex("#b0bccb"),line:hex("#303845"),blue:hex("#9abaff"),wash:hex("#24324a") } }
        else { Self { bg:hex("#ffffff"),surface:hex("#f5f6f8"),ink:hex("#1b2330"),muted:hex("#505d6f"),line:hex("#dce2e9"),blue:hex("#245cc5"),wash:hex("#eef3fd") } }
    }

}
const BODY: TextStyle = TextStyle::new(17.0,400.0).lh(28.0);
const LABEL: TextStyle = TextStyle::new(14.0,500.0).lh(20.0);
const SMALL: TextStyle = TextStyle::new(13.0,400.0).lh(20.0);
const SECTION: TextStyle = TextStyle::new(28.0,500.0).lh(36.0);

#[derive(Clone, Copy)]
enum Icon { Terminal, Code, Skill, Arrow, Right, Down, Search, Theme, Menu }

// Shape paths export as SVG and use the same geometry in the GPU view.
fn symbol(icon: Icon,size: f32,color: Color) -> El {
    use rune::{Path,Shape,LineCap,canvas};
    let path=|points: &[[f32;2]]| {
        let mut value=Path::new().move_to(points[0]);
        for point in &points[1..] {value=value.line_to(*point);}
        value
    };
    let paths=match icon {
        Icon::Terminal=>vec![path(&[[5.0,7.0],[10.0,12.0],[5.0,17.0]]),path(&[[13.0,17.0],[19.0,17.0]])],
        Icon::Code=>vec![path(&[[9.0,7.0],[4.0,12.0],[9.0,17.0]]),path(&[[15.0,7.0],[20.0,12.0],[15.0,17.0]]),path(&[[13.0,4.0],[11.0,20.0]])],
        Icon::Skill=>vec![
            path(&[[6.0,4.0],[14.0,4.0],[18.0,8.0],[18.0,20.0],[6.0,20.0],[6.0,4.0]]),
            path(&[[9.0,11.0],[15.0,11.0]]),path(&[[9.0,15.0],[15.0,15.0]]),
        ],
        Icon::Arrow=>vec![path(&[[5.0,12.0],[19.0,12.0]]),path(&[[13.0,6.0],[19.0,12.0],[13.0,18.0]])],
        Icon::Right=>vec![path(&[[10.0,7.0],[15.0,12.0],[10.0,17.0]])],
        Icon::Down=>vec![path(&[[7.0,10.0],[12.0,15.0],[17.0,10.0]])],
        Icon::Search=>vec![
            path(&[[10.0,4.0],[14.2,5.7],[16.0,10.0],[14.2,14.2],[10.0,16.0],[5.8,14.2],[4.0,10.0],[5.8,5.8],[10.0,4.0]]),
            path(&[[14.4,14.4],[20.0,20.0]]),
        ],
        Icon::Theme=>vec![
            path(&[[12.0,4.0],[17.7,6.3],[20.0,12.0],[17.7,17.7],[12.0,20.0],[6.3,17.7],[4.0,12.0],[6.3,6.3],[12.0,4.0]]),
            path(&[[12.0,4.0],[12.0,20.0]]),
        ],
        Icon::Menu=>vec![path(&[[4.0,7.0],[20.0,7.0]]),path(&[[4.0,12.0],[20.0,12.0]]),path(&[[4.0,17.0],[20.0,17.0]])],
    };
    let mut shape=Shape::new().view_box(24.0,24.0);
    for path in paths {shape=shape.stroke_path(&path,1.5,LineCap::Round,color);}
    canvas(shape).size(size,size).a11y_hidden()
}

struct State { page: Option<usize>, dark: bool, open: BTreeSet<usize>, contents_open: bool, images: Option<ImageCache>, example: usize }
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
        Self { page,dark,open,contents_open:false,images:None,example:examples::initial_step() }
    }
}
fn txt(key: &str,value: impl Into<String>,font: TextStyle,color: Color) -> El { text(value).key(key).font(font).color(color).wrap().selectable(true) }
fn rule(key: &str,p: Paint) -> El { el().key(key).w_full().h(1.0).fill(p.line).a11y_hidden() }
fn link(key: &str,label: &str,url: &str,p: Paint) -> El {
    row().key(key).href(url).focus_ring(2.0,2.0,p.blue).py(4.0).hover_group()
        .child(txt(&format!("{key}-text"),label,LABEL,p.muted).group_color(p.blue))
}
fn button(key: &str,label: &str,p: Paint) -> El {
    row().key(key).role("button").label(label).focusable(true).focus_ring(2.0,2.0,p.blue)
        .h(36.0).px(12.0).center_y().fill(p.surface).radius(8.0).hover(p.wash)
        .child(txt(&format!("{key}-text"),label,LABEL,p.ink))
}
fn header_action(key: &str, label: &str, icon: Icon, p: Paint) -> El {
    row().key(key).role("button").label(label).focusable(true).focus_ring(2.0, 2.0, p.blue)
        .size(44.0,44.0).center().radius(8.0).fill(p.bg).hover(p.surface)
        .child(symbol(icon,18.0,p.ink))
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
fn workspace_overview(s: &State, cx: &Cx<State>, width: f32, p: Paint) -> El {
    let file = "plous-overview.png";
    let url = format!("{}assets/{file}",content().base);
    col().key("workspace-screenshot").href(&url).w_full().gap(8.0)
        .focus_ring(2.0,2.0,p.blue).label("View the Plous workspace screenshot at full size").hover_group()
        .child(picture(s,cx,"workspace-image",file,width,
            "Plous terminal with the connections tree, cluster overview, and Codex assistant.")
            .w_full().aspect_ratio(3024.0/1898.0))
        .child(row().w_full().gap(8.0).justify_between().center_y()
            .child(txt("workspace-caption","Workspace overview · earlier build",SMALL,p.muted))
            .child(txt("workspace-expand","Full size",SMALL,p.blue).group_color(p.ink)))
}
fn header(s: &State,cx: &Cx<State>,phone: bool,p: Paint) -> El {
    let brand=row().key("brand").href(&content().root).label("Plous home").center_y()
        .child(picture(s,cx,"brand-wordmark",if s.dark {"lockup-dark.svg"} else {"lockup.svg"},104.0,"Plous").size(104.0,104.0*124.0/375.0));
    let current = s.page.map(|i| &content().pages[i]);
    let active = Paint { muted: p.ink, ..p };
    let primary_link=|key: &str,label: &str,destination,selected| {
        link(key,label,&format!("{}docs/{destination}",content().base),if selected {active} else {p})
            .h(64.0).px(4.0).center_y()
            .border_sides([0.0,0.0,2.0,0.0],if selected {p.blue} else {Color::TRANSPARENT})
    };
    let docs=primary_link("nav-docs","Docs","",current.is_some_and(|page| !page.developers));
    let developers=primary_link("nav-developers","Developers","contributing/",current.is_some_and(|page| page.developers));
    let nav=row().key("primary").role("navigation").label("Primary").center_y().gap(20.0)
        .child(docs).child(developers);
    let nav=if phone {nav.absolute(0.0,0.0).w(0.0).h(0.0).clip_rounded(0.0).a11y_hidden()} else {nav};
    let label=txt("search-label","Search docs",LABEL,p.muted);
    let keycap=row().center().px(6.0).h(24.0).radius(4.0).border(1.0,p.line).a11y_hidden()
        .child(txt("search-shortcut","Ctrl K",TextStyle::new(12.0,400.0).lh(16.0).mono(),p.muted));
    let (label,keycap)=if phone {
        (label.absolute(0.0,0.0).size(0.0,0.0).clip_rounded(0.0),
            keycap.absolute(0.0,0.0).size(0.0,0.0).padding(0.0).clip_rounded(0.0))
    } else { (label.w(0.0).grow(1.0), keycap) };
    let search=row().key("search").role("button").label("Search documentation").focusable(true)
        .focus_ring(2.0,2.0,p.blue).radius(8.0).hover(p.wash).gap(8.0)
        .child(symbol(Icon::Search,18.0,p.muted)).child(label).child(keycap);
    let search=if phone {
        search.size(44.0,44.0).center().fill(p.bg)
    } else {
        search.w(200.0).h(36.0).px(12.0).center_y().border(1.0,p.line).fill(p.surface)
    };
    let actions=row().center_y().gap(4.0)
        .child(search.on_press(|_: &mut State| { #[cfg(target_arch="wasm32")] open_search(); }))
        .child(header_action("theme","Change color theme",Icon::Theme,p).on_press(|s: &mut State| {
        s.dark=!s.dark;
        #[cfg(target_arch="wasm32")] set_browser_dark(s.dark);
    }));
    let menu=header_action("menu","Open navigation",Icon::Menu,p).on_press(|_: &mut State| { #[cfg(target_arch="wasm32")] open_menu(); });
    let actions=actions.child(if phone {menu} else {menu.absolute(0.0,0.0).w(0.0).h(0.0).clip_rounded(0.0).a11y_hidden()});
    row().key("header").role("banner").w_full().h(64.0).justify_between().center_y()
        .child(row().center_y().gap(32.0).child(brand).child(nav))
        .child(actions)
}
fn workflows(p: Paint, stack: bool) -> El {
    let list = if stack { col() } else { row().items_stretch().gap(32.0) };
    list.key("workflows").w_full().border_sides([1.0,0.0,0.0,0.0],p.line)
        .children(content().paths.iter().enumerate().map(|(i,path)| {
            let item = row().key(format!("workflow-{i}")).href(&path.href).focus_ring(2.0,2.0,p.blue)
                .items_start().gap(12.0).py(16.0)
                .border_sides([0.0,0.0,1.0,0.0],p.line).hover_group()
                .child(symbol(workflow_icon(path),18.0,p.muted).mt(4.0))
                .child(col().w(0.0).grow(1.0).gap(4.0)
                    .child(workflow_title(i,&path.title,p).w_full())
                    .child(txt(&format!("workflow-description-{i}"),&path.description,
                        TextStyle::new(14.0,400.0).lh(20.0),p.muted).w_full()))
                .child(symbol(Icon::Arrow,18.0,p.blue).mt(4.0));
            if stack { item.w_full() } else { item.w(0.0).grow(1.0) }
        }))
}
fn workflow_icon(path: &Link) -> Icon {
    let route = path.href.strip_prefix(&content().base).unwrap_or(&path.href);
    if route.starts_with("docs/agent-skills/") { Icon::Skill }
    else if route.starts_with("docs/mcp/") { Icon::Code }
    else { Icon::Terminal }
}
fn workflow_title(i: usize, title: &str, p: Paint) -> El {
    let mut plain = String::new();
    let mut spans = Vec::new();
    for (index, part) in title.split('`').enumerate() {
        if index % 2 == 1 { spans.push(Span::new(plain.len(), plain.len() + part.len()).mono()); }
        plain.push_str(part);
    }
    txt(&format!("workflow-title-{i}"),plain,TextStyle::new(18.0,500.0).lh(24.0),p.blue)
        .spans(spans).group_color(p.ink)
}
fn index_links(p: Paint, narrow: bool) -> El {
    let groups = content().quick_links.iter().enumerate().map(|(i, group)| {
        let items = group.items.iter().enumerate().map(|(j, item)| {
            row().key(format!("quick-link-{i}-{j}")).href(&item.href)
                .focus_ring(2.0,2.0,p.blue).w_full().min_h(36.0).center_y().py(4.0).hover_group()
                .child(txt(&format!("quick-link-label-{i}-{j}"),&item.title,
                    TextStyle::new(15.0,400.0).lh(24.0),p.blue).group_color(p.ink))
        });
        let column = col()
            .child(txt(&format!("quick-link-heading-{i}"), &group.label,
                TextStyle::new(17.0,500.0).lh(24.0),p.ink).role("heading").level(3).pb(4.0))
            .children(items);
        if narrow { column.w_full() } else { column.w(0.0).grow(1.0) }
    });
    let list = if narrow { col().gap(16.0) } else { row().items_start().gap(32.0) };
    col().key("index-links").w_full().gap(20.0).pt(24.0)
        .border_sides([1.0,0.0,0.0,0.0],p.line)
        .child(txt("quick-links", "Quick links", SECTION, p.ink).role("heading").level(2))
        .child(list.w_full().children(groups))
}
fn entry_layout(s: &State, cx: &Cx<State>, intro: El, links: El, width: f32, stack: bool, p: Paint) -> El {
    col().w_full().gap(if stack {20.0} else {28.0}).child(intro.w_full()).child(links.w_full())
        .child(workspace_overview(s,cx,width,p))
}
fn home(s: &State,cx: &Cx<State>,width: f32,p: Paint) -> El {
    let stack=width<820.0;
    let intro=col().gap(12.0)
        .child(txt("top","Kubernetes in your terminal",TextStyle::new(if stack {44.0} else {68.0},500.0)
            .lh(if stack {48.0} else {72.0}).tracking(-1.8),p.ink).w_full()
            .role("heading").level(1))
        .child(txt("home-intro","Browse cluster resources, read logs, and investigate failures in Plous.",
            TextStyle::new(18.0,400.0).lh(28.0),p.muted).w_full());
    let version=version_button(p).w(152.0);
    let intro=if stack { col().gap(16.0).child(intro.w_full()).child(version) }
        else { row().items_end().gap(32.0).child(intro.w(0.0).grow(1.0)).child(version) };
    let masthead=entry_layout(s,cx,intro,workflows(p,stack),width,stack,p)
        .pt(if stack {28.0} else {48.0}).pb(32.0);
    let mut integration_links=row().w_full().flex_wrap().gap(24.0);
    for (name,path) in [("Kubernetes","kubernetes"),("Helm","helm"),("Argo CD","argo"),("Flux","flux")] {
        integration_links=integration_links.child(link(&format!("integration-{path}"),name,&format!("{}docs/integrations/{path}/",content().base),p));
    }
    let integrations=col().w_full().gap(12.0).py(28.0)
        .child(txt("integrations-label","Integrations",SECTION,p.ink).role("heading").level(2))
        .child(integration_links);
    let contributor=col().w_full().gap(16.0).py(48.0)
        .child(txt("contribute-title","Contribute to Plous",SECTION,p.ink).role("heading").level(2))
        .child(txt("contribute-copy","Report a bug or contribute to Plous.",BODY,p.muted).max_w(640.0))
        .child(link("contributor-guide","Read the contributor guide",&format!("{}docs/contributing/",content().base),p));
    col().w_full().child(masthead).child(rule("integrations-rule",p)).child(integrations).child(rule("contribute-rule",p)).child(contributor)
}
fn version_button(p: Paint) -> El {
    button("versions",&content().version,p).label("Choose documentation version").justify_between()
        .gap(12.0).border(1.0,p.line).child(symbol(Icon::Down,16.0,p.muted))
        .on_press(|_: &mut State| {
        #[cfg(target_arch="wasm32")] open_versions();
    })
}
fn sidebar(s: &State,page: &Page,phone: bool,p: Paint) -> El {
    let index = page.route == "docs/";
    let mut nav=col().key("sidebar").role("navigation").label("Documentation navigation")
        .w(232.0).mt(40.0).pr(16.0).pb(8.0).gap(4.0)
        .border_sides([0.0,1.0,0.0,0.0],p.line);
    nav=nav.child(version_button(p).w_full().mb(12.0));
    let root = link("sidebar-root",if page.developers {"User docs"} else {"Docs home"},&format!("{}docs/",content().base),p);
    let root = root.px(12.0).min_h(36.0).center_y();
    nav=nav.child(if index { root.label("Docs, current page")
        .border_sides([0.0,0.0,0.0,2.0],p.blue).radius(8.0).fill(p.wash) } else { root });
    for (i,group) in content().navigation.iter().enumerate().filter(|(_,g)| (g.label=="Developers")==page.developers) {
        let opened=s.open.contains(&i);
        let title=button(&format!("group-toggle-{i}"),&group.label,p).w_full()
            .h(36.0).fill(p.bg)
            .justify_between().child(symbol(if opened {Icon::Down} else {Icon::Right}, 16.0, p.muted)).expanded(opened)
            .on_press(move |s: &mut State| { if !s.open.remove(&i) {s.open.insert(i);} });
        let mut items=col().key(format!("group-items-{i}")).w_full().gap(2.0).pl(8.0).pb(4.0);
        for (j,item) in group.items.iter().enumerate() {
            let selected=item.href==page.href;
            items=items.child(row().key(format!("nav-link-{i}-{j}")).href(&item.href).focus_ring(2.0,2.0,p.blue)
                .w_full().min_h(32.0).center_y().py(4.0).px(12.0).radius(4.0)
                .fill(if selected {p.wash} else {p.bg}).hover(p.surface)
                .border_sides([0.0,0.0,0.0,2.0],if selected {p.blue} else {Color::TRANSPARENT})
                .child(txt(&format!("nav-label-{i}-{j}"),&item.title,
                    TextStyle::new(13.0,if selected {500.0} else {400.0}).lh(20.0),
                    if selected {p.blue} else {p.muted}).w(0.0).grow(1.0)));
        }
        if !opened {
            items=items.h(0.0).clip_rounded(0.0).a11y_hidden().absolute(0.0,0.0);
        }
        nav=nav.child(title).child(items);
    }
    if phone {nav.absolute(0.0,0.0).w(0.0).h(0.0).padding(0.0).mt(0.0).clip_rounded(0.0).a11y_hidden()} else {nav}
}
fn markdown(s: &State,cx: &Cx<State>,page: &Page,width: f32,p: Paint) -> El {
    #[cfg(target_arch="wasm32")]
    let source=browser_markdown(&page.body);
    #[cfg(not(target_arch="wasm32"))]
    let source=page.body.clone();
    let mut style=MarkdownStyle::neutral(s.dark).text(p.ink).muted(p.muted).link(p.blue).rule(p.line)
        .body_font(BODY).heading_font(2,SECTION).strong_weight(600.0)
        .heading_font(3,TextStyle::new(20.0,500.0).lh(28.0))
        .inline_code_bg(p.surface).code_block(Color::TRANSPARENT,Color::TRANSPARENT)
        .code_block_font(TextStyle::new(14.0,400.0).lh(24.0).mono()).code_block_padding(16.0,12.0)
        .table_font(TextStyle::new(15.0,400.0).lh(24.0)).table_cell_padding(12.0,8.0)
        .table_full_width(true).table_header_fill(p.surface).table_header_text(p.ink)
        .table_rules(p.line).text_width(width).wide_fade(0.0);
    if page.route == "docs/reference/keymap/" {
        style = style.inline_code_bg(p.surface).inline_code_pad(4.0).inline_code_font(0.92, 500.0)
            .table_cell_padding(12.0,12.0).table_header_weight(500.0);
    }
    if page.route == "docs/agent-skills/plous-debug/" {
        style = style.table_cell_padding(8.0,8.0).inline_code_font(0.85,400.0);
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
            let shell = caption == "Shell command";
            let mut title = row().center_y().gap(8.0);
            if shell { title = title.child(symbol(Icon::Terminal,16.0,p.blue)); }
            let title = title.child(txt(&format!("language-{}",code.index),caption,
                TextStyle::new(12.0,500.0).lh(16.0).mono(),p.muted));
            col().role("region").label(format!("{language} code example")).w_full()
                .fill(p.bg)
                .border(1.0,p.line).radius(8.0).clip_rounded(8.0)
                .child(row().w_full().pl(16.0).pr(8.0).py(4.0).justify_between().center_y()
                    .fill(p.surface)
                    .border_sides([0.0,0.0,1.0,0.0],p.line)
                    .child(title)
                    .child(button(&format!("copy-{}",code.index),"Copy",p).h(28.0).radius(4.0)
                        .fill(p.bg).hover(p.wash)
                        .label(format!("Copy code block {}",code.index+1))
                        .on_press_tx(move |_: &mut State,tx| tx.copy(value.clone()))))
                .child(card)
        });
    if page.examples.is_empty() {
        return md.render(&style);
    }
    let examples = std::cell::RefCell::new(page.examples.iter().map(|example| {
        let content = match example {
            Example::PanelFocus => examples::panels(s,cx,p),
            Example::WorkspaceOverview => workspace_overview(s,cx,width,p),
            Example::McpTarget => examples::target(s,cx,p),
        };
        (example.marker(), content)
    }).collect::<BTreeMap<_,_>>());
    md.quote_frame(move |quote, frame| examples.borrow_mut().remove(frame.text).unwrap_or(quote)).render(&style)
}
fn index_install(p: Paint) -> El {
    let value = &content().install_command;
    let copy = button("copy-index-install","Copy",p).h(32.0)
        .label("Copy code block 1")
        .on_press_tx(move |_: &mut State,tx| tx.copy(value.clone()));
    col().key("index-install").role("region").label("Install with Homebrew")
        .w_full().gap(8.0)
        .child(txt("install-heading","Install with Homebrew",LABEL,p.ink))
        .child(row().w_full().gap(8.0).center_y().px(8.0).py(4.0)
            .fill(p.surface).border(1.0,p.line).radius(8.0)
            .child(txt("install-command",value,TextStyle::new(13.0,500.0).lh(20.0).mono(),p.ink).w(0.0).grow(1.0))
            .child(copy.fill(p.bg)))
        .child(link("install-methods","Other install methods",
            &format!("{}docs/getting-started/installation/",content().base),p))
}
fn docs_index(s: &State, cx: &Cx<State>, page: &Page, width: f32, stack: bool, p: Paint) -> El {
    let title = TextStyle::new(if stack {52.0} else {72.0},500.0)
        .lh(if stack {56.0} else {76.0}).tracking(if stack {-1.4} else {-2.4});
    let intro = col().gap(12.0)
        .child(txt("top",&page.title,title,p.ink).role("heading").level(1).w_full())
        .child(txt("index-intro",&content().index_intro,TextStyle::new(18.0,400.0).lh(28.0),p.muted)
            .w_full());
    let install = index_install(p);
    let intro = if stack { col().gap(20.0).child(intro.w_full()).child(install) }
        else { row().items_end().gap(40.0)
            .child(intro.w(0.0).grow(3.0)).child(install.w(0.0).grow(2.0)) };
    let actions = col().w_full().gap(8.0)
        .child(txt("choose-your-workflow","Choose your workflow",LABEL,p.ink).role("heading").level(2))
        .child(workflows(p,stack));
    col().key("article").w_full().max_w(1040.0).gap(if stack {32.0} else {40.0}).py(if stack {28.0} else {40.0})
        .child(entry_layout(s,cx,intro,actions,width.min(1040.0),stack,p))
        .child(index_links(p, stack))
        .child(col().w_full().max_w(640.0).pt(16.0).border_sides([1.0,0.0,0.0,0.0],p.line)
            .child(markdown(s,cx,page,width.min(640.0),p)))
        .child(col().w_full().gap(8.0).child(rule("edit-rule",p)).child(edit_link(page,p)))
}
fn edit_link(page: &Page, p: Paint) -> El {
    link("edit-source",if content().version=="Development" {"Edit this page on GitHub"} else {"View source"},&page.edit,p)
}
fn article(s: &State,cx: &Cx<State>,page: &Page,narrow: bool,wide: bool,p: Paint) -> El {
    let width = cx.layout_size("article").map_or(if narrow {320.0} else {704.0}, |v| v[0]);
    let mut header = col().w_full().gap(8.0);
    if page.section != page.title {
        let trail = row().center_y().gap(8.0)
            .child(link("section-docs","Docs",&format!("{}docs/",content().base),p))
            .child(symbol(Icon::Right,16.0,p.muted))
            .child(txt("section-name",&page.section,LABEL,p.ink));
        let mut context = row().w_full().justify_between().center_y().gap(8.0).child(trail);
        if narrow {
            context = context.child(txt("guide-version",&content().version,SMALL,p.muted));
        }
        header = header.child(context);
    }
    header = header.child(txt("top",&page.title,TextStyle::new(if narrow {36.0} else {48.0},500.0)
        .lh(if narrow {40.0} else {52.0}).tracking(if narrow {-0.6} else {-1.3}),p.ink).role("heading").level(1));
    let has_contents = page.headings.iter().any(|heading| heading.level == 2);
    let header = if wide || !has_contents {
        header.pb(20.0).border_sides([0.0,0.0,1.0,0.0],p.line)
    } else { header };
    let header = if has_contents { header.child(inline_contents(s,page,wide,p)) } else { header };
    col().key("article").w_full().max_w(704.0).gap(24.0)
        .py(if narrow {24.0} else {40.0}).child(header)
        .child(markdown(s,cx,page,width,p))
        .child(rule("edit-rule",p)).child(edit_link(page,p))
}
fn contents_links(page: &Page, key: &str, p: Paint) -> El {
    col().key(key).role("navigation").label("On this page").w_full()
        .children(page.headings.iter().filter(|heading| heading.level == 2).map(|heading| {
            let id = format!("{key}-{}", heading.id);
            row().key(&id).href(format!("#{}",heading.id)).focus_ring(2.0,2.0,p.blue)
                .w_full().py(4.0).hover_group()
                .child(txt(&format!("{id}-text"),&heading.title,TextStyle::new(14.0,400.0).lh(20.0),p.muted)
                    .w(0.0).grow(1.0).group_color(p.blue))
        }))
}
fn inline_contents(s: &State, page: &Page, wide: bool, p: Paint) -> El {
    let toggle = button("contents-toggle","On this page",p).w_full().justify_between().h(40.0)
        .px(0.0).fill(p.bg).hover(p.surface)
        .child(symbol(if s.contents_open {Icon::Down} else {Icon::Right}, 16.0, p.muted)).expanded(s.contents_open)
        .on_press(|s: &mut State| s.contents_open = !s.contents_open);
    let links = contents_links(page,"contents-inline-links",p).pb(8.0);
    #[cfg(target_arch="wasm32")]
    let links = if s.contents_open { links } else { links.h(0.0).padding(0.0).clip_rounded(0.0).a11y_hidden() };
    let panel = col().key("contents-inline").role("region").w_full().mt(8.0)
        .border_sides([1.0,0.0,1.0,0.0],p.line)
        .child(toggle).child(links);
    if wide { panel.h(0.0).clip_rounded(0.0).a11y_hidden() } else { panel }
}
fn toc(page: &Page,wide: bool,p: Paint) -> El {
    let panel = col().key("contents").role("region").w(224.0).mt(40.0).pl(20.0).gap(8.0)
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
    let frame_width=1376.0;
    let frame=|e: El| e.w_full().max_w(frame_width).px(gutter);
    let main=if let Some(index)=s.page {
        let page=&content().pages[index];
        let columns=(if phone {col()} else {row().items_start()}).w_full().gap(if phone {0.0} else {40.0});
        let has_contents = page.route != "docs/" && page.headings.iter().any(|heading| heading.level == 2);
        let wide = measured >= 1280.0 && has_contents;
        let document = if page.route == "docs/" {
            let width = measured.min(frame_width) - 2.0 * gutter - if phone {0.0} else {272.0};
            docs_index(s,cx,page,width,width < 820.0,p)
        } else { article(s,cx,page,narrow,wide,p) };
        let content_columns = row().items_start().gap(if wide {32.0} else {0.0})
            .child(document.w(0.0).grow(1.0));
        let content_columns = if has_contents { content_columns.child(toc(page,wide,p)) } else { content_columns };
        let content_columns=if phone {content_columns.w_full()} else {content_columns.w(0.0).grow(1.0)};
        columns.child(sidebar(s,page,phone,p)).child(content_columns)
    } else {home(s,cx,measured.min(frame_width) - 2.0 * gutter,p)};
    let footer=col().w_full().gap(16.0).pt(24.0).pb(32.0).child(rule("footer-rule",p))
        .child((if narrow {col().gap(12.0)} else {row().justify_between().center_y()})
            .child(txt("footer-note","Plous · Open source · Early development",SMALL,p.muted))
            .child(row().gap(24.0).child(link("footer-developers","Developers",&format!("{}docs/contributing/",content().base),p)).child(link("footer-github","GitHub","https://github.com/simjay/plous",p))));
    col().key("page").w_full().fill(p.bg).items_center()
        .child(frame(header(s,cx,phone,p))).child(rule("header-rule",p))
        .child(frame(main).key("main").role("main")).child(frame(footer))
}
#[cfg_attr(target_arch="wasm32",wasm_bindgen::prelude::wasm_bindgen(start))]
pub fn start() {
    let initial=State::new();
    let title=initial.page.map_or("Plous documentation",|i|content().pages[i].title.as_str());
    let spec=AppSpec { families:("Geist","Geist Mono"),setup:Some(|s: &mut State,w|s.images=Some(ImageCache::new(w))),..AppSpec::new(title,(1440.0,1000.0),State::new,view) };
    let app = rune::App::new(spec).web_document().font_url(&asset("geist.ttf")).font_url(&asset("geist-mono.ttf"));
    let app = if initial.page.is_some_and(|i| content().pages[i].route == "docs/") {
        app.font_url(&asset("Inter-Regular.ttf")).font_url(&asset("Inter-SemiBold.ttf"))
    } else { app };
    app.run();
}
