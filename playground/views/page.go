package views

import (
	"github.com/dave/jsgo/playground/actions"
	"github.com/dave/jsgo/playground/stores"
	"github.com/dave/splitter"
	"github.com/gopherjs/gopherjs/js"
	"github.com/gopherjs/vecty"
	"github.com/gopherjs/vecty/elem"
	"github.com/gopherjs/vecty/event"
	"github.com/gopherjs/vecty/prop"
	"honnef.co/go/js/dom"
)

type Page struct {
	vecty.Core
	app *stores.App

	Sizes []float64 `vecty:"prop"`

	newItemTitle  string
	left, right   *vecty.HTML
	split         *splitter.Split
	compileButton *vecty.HTML
	optionsButton *vecty.HTML
	frame         *vecty.HTML
}

func NewPage(app *stores.App) *Page {
	v := &Page{
		app: app,
	}
	return v
}

func (v *Page) Mount() {
	v.app.Watch(v, func(done chan struct{}) {
		defer close(done)
		v.Sizes = v.app.Editor.Sizes()
		v.split.SetSizesIfChanged(v.Sizes)

		// Only top-level page should fire vecty.Rerender
		vecty.Rerender(v)
	})

	v.split = splitter.New("split")
	v.split.Init(
		js.S{"#left", "#right"},
		js.M{
			"sizes": v.Sizes,
			"onDragEnd": func() {
				v.app.Dispatch(&actions.UserChangedSplit{
					Sizes: v.split.GetSizes(),
				})
			},
		},
	)
}

func (v *Page) Unmount() {
	v.app.Delete(v)
}

func (v *Page) onInject(event *vecty.Event) {
	doc := dom.GetWindow().Document()
	v.frame.Node().Set("innerHTML", "")
	script := doc.CreateElement("script")
	script.SetInnerHTML("alert('foo');")
	v.frame.Node().Call("appendChild", script)
}

func (v *Page) onUpdate(event *vecty.Event) {
	v.app.Dispatch(&actions.UpdateStart{})
}

func (v *Page) onCompile(event *vecty.Event) {
	v.app.Dispatch(&actions.CompileStart{})
}

func (v *Page) Render() vecty.ComponentOrHTML {

	v.left = v.renderLeft()
	v.right = v.renderRight()

	return elem.Body(
		elem.Div(
			vecty.Markup(
				vecty.Class("container-fluid", "p-0", "split", "split-horizontal"),
			),
			v.left,
			v.right,
		),
	)
}

func (v *Page) renderLeft() *vecty.HTML {
	return elem.Div(
		vecty.Markup(
			prop.ID("left"),
			vecty.Class("split"),
		),
		v.renderHeader(),
		NewEditor(v.app),
	)
}

func (v *Page) renderHeader() *vecty.HTML {

	var buttonText string
	var buttonAction func(*vecty.Event)
	var buttonDisabled bool

	if v.app.Archive.Updating() {
		buttonText = "Updating..."
		buttonAction = nil
		buttonDisabled = true
	} else if v.app.Archive.Fresh(v.app.Scanner.Imports()) {
		buttonText = "Compile"
		buttonAction = v.onCompile
		buttonDisabled = false
	} else {
		buttonText = "Update"
		buttonAction = v.onUpdate
		buttonDisabled = false
	}

	return elem.Navigation(
		vecty.Markup(
			vecty.Class("navbar", "navbar-expand", "navbar-light", "bg-light"),
		),
		elem.Div(
			vecty.Markup(
				prop.ID("navbarSupportedContent"),
				vecty.Class("collapse", "navbar-collapse"),
			),
		),
		elem.UnorderedList(
			vecty.Markup(
				vecty.Class("navbar-nav", "ml-auto"),
			),
			elem.ListItem(
				vecty.Markup(
					vecty.Class("nav-item", "btn-group"),
				),
				elem.Button(
					vecty.Markup(
						vecty.Property("type", "button"),
						vecty.Class("btn", "btn-primary"),
						event.Click(buttonAction).PreventDefault(),
						vecty.Property("disabled", buttonDisabled),
					),
					vecty.Text(buttonText),
				),
				elem.Button(
					vecty.Markup(
						vecty.Property("type", "button"),
						vecty.Data("toggle", "dropdown"),
						vecty.Property("aria-haspopup", "true"),
						vecty.Property("aria-expanded", "false"),
						vecty.Class("btn", "btn-primary", "dropdown-toggle", "dropdown-toggle-split"),
					),
					elem.Span(vecty.Markup(vecty.Class("sr-only")), vecty.Text("Options")),
				),
				elem.Div(
					vecty.Markup(
						vecty.Class("dropdown-menu", "dropdown-menu-right"),
					),
					elem.Anchor(
						vecty.Markup(
							vecty.Class("dropdown-item"),
							prop.Href("#"),
							event.Click(v.onUpdate).PreventDefault(),
						),
						vecty.Text("Update"),
					),
					/*
						elem.Anchor(
							vecty.Markup(
								vecty.Class("dropdown-item"),
								prop.Href("#"),
								event.Click(func(e *vecty.Event) {
									js.Global.Call("alert", "TODO")
								}).StopPropagation(),
							),
							elem.Input(
								vecty.Markup(
									prop.Type(prop.TypeCheckbox),
									vecty.Class("form-check-input", "dropdown-item"),
									prop.ID("dropdownCheckDeps"),
								),
							),
							elem.Label(
								vecty.Markup(
									vecty.Class("form-check-label"),
									prop.For("dropdownCheckDeps"),
								),
								vecty.Text("Update imports"),
							),
						),
					*/
					elem.Div(
						vecty.Markup(
							vecty.Class("dropdown-divider"),
						),
					),
					elem.Anchor(
						vecty.Markup(
							vecty.Class("dropdown-item"),
							prop.Href("#"),
							event.Click(func(e *vecty.Event) {
								js.Global.Call("alert", "TODO")
							}).PreventDefault(),
						),
						vecty.Text("Format code"),
					),
					elem.Div(
						vecty.Markup(
							vecty.Class("dropdown-divider"),
						),
					),
					elem.Anchor(
						vecty.Markup(
							vecty.Class("dropdown-item"),
							prop.Href("#"),
							event.Click(func(e *vecty.Event) {
								js.Global.Call("alert", "TODO")
							}).PreventDefault(),
						),
						vecty.Text("Build tags..."),
					),
					elem.Anchor(
						vecty.Markup(
							vecty.Class("dropdown-item"),
							prop.Href("#"),
							event.Click(func(e *vecty.Event) {
								js.Global.Call("alert", "TODO")
							}).PreventDefault(),
						),
						vecty.Text("Save"),
					),
					elem.Anchor(
						vecty.Markup(
							vecty.Class("dropdown-item"),
							prop.Href("#"),
							event.Click(func(e *vecty.Event) {
								js.Global.Call("alert", "TODO")
							}).PreventDefault(),
						),
						vecty.Text("Deploy"),
					),
				),
			),
		),
	)
}

func (v *Page) renderRight() *vecty.HTML {
	v.frame = elem.InlineFrame(
		vecty.Markup(
			prop.ID("iframe"),
			vecty.Style("width", "100%"),
			vecty.Style("height", "100%"),
			vecty.Style("border", "0"),
		),
	)
	return elem.Div(
		vecty.Markup(
			prop.ID("right"),
			vecty.Class("split"),
		),
		v.frame,
	)
}
