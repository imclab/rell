// Package viewexamples provides the various views on Rell examples.
package viewexamples

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/daaku/go.counting"
	"github.com/daaku/go.errcode"
	"github.com/daaku/go.fburl"
	"github.com/daaku/go.h"
	"github.com/daaku/go.h.js.fb"
	"github.com/daaku/go.h.js.loader"
	"github.com/daaku/go.h.ui"
	"github.com/daaku/go.htmlwriter"
	"github.com/daaku/go.static"
	"github.com/daaku/go.stats"
	"github.com/daaku/go.xsrf"
	"github.com/daaku/sortutil"

	"github.com/daaku/rell/context"
	"github.com/daaku/rell/examples"
	"github.com/daaku/rell/js"
	"github.com/daaku/rell/view"
)

const (
	savedPath = "/saved/"
	paramName = "-xsrf-token-"
)

var (
	envOptions = map[string]string{
		"":               "Production with CDN",
		fburl.Production: "Production without CDN",
		fburl.Beta:       "Beta",
		"latest":         "Latest",
		"dev":            "Dev",
		"intern":         "Intern",
		"inyour":         "In Your",
		"sb":             "Sandbox",
	}
	viewModeOptions = map[string]string{
		context.Website: "Website",
		context.PageTab: "Page Tab",
		context.Canvas:  "Canvas",
	}
	errTokenMismatch = errcode.New(http.StatusForbidden, "Token mismatch.")
)

type Handler struct {
	ContextParser *context.Parser
	ExampleStore  *examples.Store
	Static        *static.Handler
	Stats         stats.Backend
	Xsrf          *xsrf.Provider
}

// Parse the Context and an Example.
func (h *Handler) parse(r *http.Request) (*context.Context, *examples.Example, error) {
	context, err := h.ContextParser.FromRequest(r)
	if err != nil {
		return nil, nil, err
	}
	example, err := h.ExampleStore.Load(context.Version, r.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	return context, example, nil
}

func (a *Handler) List(w http.ResponseWriter, r *http.Request) {
	context, err := a.ContextParser.FromRequest(r)
	if err != nil {
		view.Error(w, r, a.Static, err)
		return
	}
	a.Stats.Count("viewed examples listing", 1)
	h.WriteResponse(w, r, &examplesList{
		Context: context,
		Static:  a.Static,
		DB:      examples.GetDB(context.Version),
	})
}

func (a *Handler) Saved(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" && r.URL.Path == savedPath {
		c, err := a.ContextParser.FromRequest(r)
		if err != nil {
			view.Error(w, r, a.Static, err)
			return
		}
		if !a.Xsrf.Validate(r.FormValue(paramName), w, r, savedPath) {
			a.Stats.Count(savedPath+" xsrf failure", 1)
			view.Error(w, r, a.Static, errTokenMismatch)
			return
		}
		content := bytes.TrimSpace([]byte(r.FormValue("code")))
		content = bytes.Replace(content, []byte{13}, nil, -1) // remove CR
		id := examples.ContentID(content)
		db := examples.GetDB(c.Version)
		example, ok := db.Reverse[id]
		if ok {
			http.Redirect(w, r, c.ViewURL(example.URL), 302)
			return
		}
		err = a.ExampleStore.Save(id, content)
		if err != nil {
			view.Error(w, r, a.Static, err)
			return
		}
		a.Stats.Count("saved example", 1)
		http.Redirect(w, r, c.ViewURL(savedPath+id), 302)
		return
	} else {
		context, example, err := a.parse(r)
		if err != nil {
			view.Error(w, r, a.Static, err)
			return
		}
		a.Stats.Count("viewed saved example", 1)
		h.WriteResponse(w, r, &page{
			Writer:        w,
			Request:       r,
			ContextParser: a.ContextParser,
			Context:       context,
			Static:        a.Static,
			Example:       example,
			Xsrf:          a.Xsrf,
		})
	}
}

func (a *Handler) Raw(w http.ResponseWriter, r *http.Request) {
	context, example, err := a.parse(r)
	if err != nil {
		view.Error(w, r, a.Static, err)
		return
	}
	if !example.AutoRun {
		view.Error(
			w, r, a.Static, errors.New("Not allowed to view this example in raw mode."))
		return
	}
	a.Stats.Count("viewed example in raw mode", 1)
	h.WriteResponse(w, r, &exampleContent{
		ContextParser: a.ContextParser,
		Context:       context,
		Example:       example,
	})
}

func (a *Handler) Simple(w http.ResponseWriter, r *http.Request) {
	context, example, err := a.parse(r)
	if err != nil {
		view.Error(w, r, a.Static, err)
		return
	}
	if !example.AutoRun {
		view.Error(
			w, r, a.Static, errors.New("Not allowed to view this example in simple mode."))
		return
	}
	a.Stats.Count("viewed example in simple mode", 1)
	h.WriteResponse(w, r, &h.Document{
		Inner: &h.Frag{
			&h.Head{
				Inner: &h.Frag{
					&h.Meta{Charset: "utf-8"},
					&h.Title{h.String(example.Title)},
				},
			},
			&h.Body{
				Inner: &h.Frag{
					&loader.HTML{
						Resource: []loader.Resource{
							&fb.Init{
								AppID:      context.AppID,
								ChannelURL: context.ChannelURL(),
								URL:        context.SdkURL(),
							},
						},
					},
					&h.Div{
						ID: "example",
						Inner: &exampleContent{
							ContextParser: a.ContextParser,
							Context:       context,
							Example:       example,
						},
					},
				},
			},
		},
	})
}

func (a *Handler) SdkChannel(w http.ResponseWriter, r *http.Request) {
	const maxAge = 31536000 // 1 year
	context, err := a.ContextParser.FromRequest(r)
	if err != nil {
		view.Error(w, r, a.Static, err)
		return
	}
	a.Stats.Count("viewed channel", 1)
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.WriteResponse(w, r, &h.Script{Src: context.SdkURL()})
}

func (a *Handler) Example(w http.ResponseWriter, r *http.Request) {
	context, example, err := a.parse(r)
	if err != nil {
		view.Error(w, r, a.Static, err)
		return
	}
	a.Stats.Count("viewed stock example", 1)
	h.WriteResponse(w, r, &page{
		Writer:        w,
		Request:       r,
		ContextParser: a.ContextParser,
		Context:       context,
		Static:        a.Static,
		Example:       example,
		Xsrf:          a.Xsrf,
	})
}

type page struct {
	Writer        http.ResponseWriter
	Request       *http.Request
	ContextParser *context.Parser
	Context       *context.Context
	Static        *static.Handler
	Example       *examples.Example
	Xsrf          *xsrf.Provider
}

func (p *page) HTML() (h.HTML, error) {
	return &view.Page{
		Context: p.Context,
		Static:  p.Static,
		Title:   p.Example.Title,
		Class:   "main",
		Resource: []loader.Resource{&js.Init{
			Context: p.Context,
			Example: p.Example,
		}},
		Body: &h.Div{
			Class: "container-fluid",
			Inner: &h.Frag{
				&h.Form{
					Action: savedPath,
					Method: h.Post,
					Target: "_top",
					Inner: &h.Frag{
						h.HiddenInputs(url.Values{
							paramName: []string{p.Xsrf.Token(p.Writer, p.Request, savedPath)},
						}),
						&h.Div{
							Class: "row-fluid",
							Inner: &h.Frag{
								&h.Div{
									Class: "span8",
									Inner: &h.Frag{
										&editorTop{Context: p.Context, Example: p.Example},
										&editorArea{
											ContextParser: p.ContextParser,
											Context:       p.Context,
											Example:       p.Example,
										},
										&editorBottom{Context: p.Context, Example: p.Example},
									},
								},
								&h.Div{
									Class: "span4",
									Inner: &h.Frag{
										&contextEditor{Context: p.Context, Example: p.Example},
										&logContainer{},
									},
								},
							},
						},
					},
				},
				&h.Div{
					Class: "row-fluid",
					Inner: &h.Div{
						Class: "span12",
						Inner: &editorOutput{},
					},
				},
			},
		},
	}, nil
}

type editorTop struct {
	Context *context.Context
	Example *examples.Example
}

func (e *editorTop) HTML() (h.HTML, error) {
	left := &h.Frag{
		&h.A{
			ID: "rell-login",
			Inner: &h.Span{
				Inner: h.String(" Log In"),
			},
		},
		h.String(" "),
		&h.Span{ID: "auth-status-label", Inner: h.String("Status:")},
		h.String(" "),
		&h.Span{ID: "auth-status", Inner: h.String("waiting")},
		h.String(" "),
		&h.Span{Class: "bar", Inner: h.String("|")},
		h.String(" "),
		&h.A{
			ID:    "rell-disconnect",
			Inner: h.String("Disconnect"),
		},
		h.String(" "),
		&h.Span{Class: "bar", Inner: h.String("|")},
		h.String(" "),
		&h.A{
			ID:    "rell-logout",
			Inner: h.String("Logout"),
		},
	}

	if e.Context.IsEmployee {
		return &h.Div{
			Class: "row-fluid form-inline",
			Inner: &h.Frag{
				&h.Div{
					Class: "span8",
					Inner: left,
				},
				&h.Div{
					Class: "span4",
					Inner: &h.Div{
						Class: "pull-right",
						Inner: &envSelector{
							Context: e.Context,
							Example: e.Example,
						},
					},
				},
			},
		}, nil
	}
	return &h.Div{
		Class: "row-fluid form-inline",
		Inner: &h.Frag{
			&h.Div{
				Class: "span12",
				Inner: left,
			},
		},
	}, nil
}

type editorArea struct {
	ContextParser *context.Parser
	Context       *context.Context
	Example       *examples.Example
}

func (e *editorArea) HTML() (h.HTML, error) {
	return &h.Div{
		Class: "row-fluid",
		Inner: &h.Textarea{
			ID:   "jscode",
			Name: "code",
			Inner: &exampleContent{
				ContextParser: e.ContextParser,
				Context:       e.Context,
				Example:       e.Example,
			},
		},
	}, nil
}

type viewModeDropdown struct {
	Context *context.Context
	Example *examples.Example
}

func (d *viewModeDropdown) HTML() (h.HTML, error) {
	return &h.Div{
		Class: "btn-group",
		Inner: &h.Frag{
			&h.Button{
				Class: "btn",
				Inner: &h.Frag{
					&h.I{Class: "icon-eye-open"},
					h.String(" "),
					h.String(viewModeOptions[d.Context.ViewMode]),
				},
			},
			&h.Button{
				Class: "btn dropdown-toggle",
				Data: map[string]interface{}{
					"toggle": "dropdown",
				},
				Inner: &h.Span{
					Class: "caret",
				},
			},
			&h.Ul{
				Class: "dropdown-menu",
				Inner: &h.Frag{
					&h.Li{
						Inner: &h.A{
							Inner:  h.String(viewModeOptions[context.Website]),
							Target: "_top",
							HREF:   d.Context.URL(d.Example.URL).String(),
						},
					},
					&h.Li{
						Inner: &h.A{
							Inner:  h.String(viewModeOptions[context.Canvas]),
							Target: "_top",
							HREF:   d.Context.CanvasURL(d.Example.URL),
						},
					},
					&h.Li{
						Inner: &h.A{
							Inner:  h.String(viewModeOptions[context.PageTab]),
							Target: "_top",
							HREF:   d.Context.PageTabURL(d.Example.URL),
						},
					},
				},
			},
			&h.Div{
				Style: "display:none",
				Inner: &h.Input{
					Type:  "hidden",
					ID:    "rell-view-mode",
					Name:  "view-mode",
					Value: d.Context.ViewMode,
				},
			},
		},
	}, nil
}

type editorBottom struct {
	Context *context.Context
	Example *examples.Example
}

func (e *editorBottom) HTML() (h.HTML, error) {
	runButton := &h.A{
		ID:    "rell-run-code",
		Class: "btn btn-primary",
		Inner: &h.Frag{
			&h.I{Class: "icon-play icon-white"},
			h.String(" Run Code"),
		},
	}
	if !e.Example.AutoRun {
		runButton.Rel = "popover"
		runButton.Data = map[string]interface{}{
			"title":     "Click to Run",
			"content":   "This example does not run automatically. Click this button to run it.",
			"placement": "top",
			"trigger":   "manual",
		}
	}
	return &h.Div{
		Class: "row-fluid form-inline",
		Inner: &h.Frag{
			&h.Strong{
				Class: "span4",
				Inner: &h.A{
					HREF:  e.Context.URL("/examples/").String(),
					Inner: h.String("Examples"),
				},
			},
			&h.Div{
				Class: "span8",
				Inner: &h.Div{
					Class: "btn-toolbar pull-right",
					Inner: &h.Frag{
						&viewModeDropdown{
							Context: e.Context,
							Example: e.Example,
						},
						h.String(" "),
						&h.Div{
							Class: "btn-group",
							Inner: &h.Button{
								Class: "btn",
								Type:  "submit",
								Inner: &h.Frag{
									&h.I{Class: "icon-file"},
									h.String(" Save Code"),
								},
							},
						},
						h.String(" "),
						&h.Div{
							Class: "btn-group",
							Inner: runButton,
						},
					},
				},
			},
		},
	}, nil
}

type editorOutput struct{}

func (e *editorOutput) HTML() (h.HTML, error) {
	return &h.Div{Class: "row-fluid", ID: "jsroot"}, nil
}

type logContainer struct{}

func (e *logContainer) HTML() (h.HTML, error) {
	return &h.Div{
		ID: "log-container",
		Inner: &h.Frag{
			&h.Button{
				ID:    "rell-log-clear",
				Class: "btn",
				Inner: h.String("Clear"),
			},
			&h.Div{ID: "log"},
		},
	}, nil
}

type contextEditor struct {
	Context *context.Context
	Example *examples.Example
}

func (e *contextEditor) HTML() (h.HTML, error) {
	if !e.Context.IsEmployee {
		return h.HiddenInputs(e.Context.Values()), nil
	}
	return &h.Div{
		Class: "well form-horizontal",
		Inner: &h.Frag{
			&ui.TextInput{
				Label:      h.String("Application ID"),
				Name:       "appid",
				Value:      e.Context.AppID,
				InputClass: "input-medium",
				Tooltip:    "Make sure the base domain in the application settings for the specified ID allows fbrell.com.",
			},
			&ui.ToggleGroup{
				Inner: &h.Frag{
					&ui.ToggleItem{
						Name:        "init",
						Checked:     e.Context.Init,
						Description: h.String("Automatically initialize SDK."),
						Tooltip:     "This controls if FB.init() is automatically called. If off, you'll need to call it in your code.",
					},
					&ui.ToggleItem{
						Name:        "status",
						Checked:     e.Context.Status,
						Description: h.String("Automatically trigger status ping."),
						Tooltip:     "This controls the \"status\" parameter to FB.init.",
					},
					&ui.ToggleItem{
						Name:        "channel",
						Checked:     e.Context.UseChannel,
						Description: h.String("Specify explicit XD channel."),
						Tooltip:     "If enabled, the FB.init() call will get a custom \"channelUrl\" parameter pointed to " + e.Context.AbsoluteURL("/channel/").String(),
					},
					&ui.ToggleItem{
						Name:        "frictionlessRequests",
						Checked:     e.Context.FrictionlessRequests,
						Description: h.String("Enable frictionless requests."),
						Tooltip:     "This controls the \"frictionlessRequests\" parameter to FB.init.",
					},
				},
			},
			&h.Div{
				Class: "form-actions",
				Inner: &h.Frag{
					&h.Button{
						Type:  "submit",
						Class: "btn btn-primary",
						Inner: &h.Frag{
							&h.I{Class: "icon-refresh icon-white"},
							h.String(" Update"),
						},
					},
				},
			},
		},
	}, nil
}

type examplesList struct {
	Context *context.Context
	DB      *examples.DB
	Static  *static.Handler
}

func (l *examplesList) HTML() (h.HTML, error) {
	categories := &h.Frag{}
	for _, category := range l.DB.Category {
		if !category.Hidden {
			categories.Append(&exampleCategory{
				Context:  l.Context,
				Category: category,
			})
		}
	}
	return &view.Page{
		Context: l.Context,
		Static:  l.Static,
		Title:   "Examples",
		Class:   "examples",
		Body: &h.Div{
			Class: "container",
			Inner: &h.Div{
				Class: "row",
				Inner: &h.Div{
					Class: "span12",
					Inner: &h.Frag{
						&h.H1{Inner: h.String("Examples")},
						categories,
					},
				},
			},
		},
	}, nil
}

type exampleCategory struct {
	Context  *context.Context
	Category *examples.Category
}

func (c *exampleCategory) HTML() (h.HTML, error) {
	li := &h.Frag{}
	for _, example := range c.Category.Example {
		li.Append(&h.Li{
			Inner: &h.A{
				HREF:  c.Context.URL(example.URL).String(),
				Inner: h.String(example.Name),
			},
		})
	}
	return &h.Frag{
		&h.H2{Inner: h.String(c.Category.Name)},
		&h.Ul{Inner: li},
	}, nil
}

type envSelector struct {
	Context *context.Context
	Example *examples.Example
}

func (e *envSelector) HTML() (h.HTML, error) {
	if !e.Context.IsEmployee {
		return nil, nil
	}
	frag := &h.Frag{
		h.HiddenInputs(url.Values{
			"server": []string{e.Context.Env},
		}),
	}
	for _, pair := range sortutil.StringMapByValue(envOptions) {
		if e.Context.Env == pair.Key {
			continue
		}
		ctxCopy := e.Context.Copy()
		ctxCopy.Env = pair.Key
		frag.Append(&h.Li{
			Inner: &h.A{
				Inner:  h.String(pair.Value),
				Target: "_top",
				HREF:   ctxCopy.ViewURL(e.Example.URL),
			},
		})
	}

	title := envOptions[e.Context.Env]
	if title == "" {
		title = e.Context.Env
	}
	return &h.Div{
		Class: "btn-group",
		Inner: &h.Frag{
			&h.Button{
				Class: "btn",
				Inner: &h.Frag{
					&h.I{Class: "icon-road"},
					h.String(" "),
					h.String(title),
				},
			},
			&h.Button{
				Class: "btn dropdown-toggle",
				Data: map[string]interface{}{
					"toggle": "dropdown",
				},
				Inner: &h.Span{
					Class: "caret",
				},
			},
			&h.Ul{
				Class: "dropdown-menu",
				Inner: frag,
			},
		},
	}, nil
}

type exampleContent struct {
	ContextParser *context.Parser
	Context       *context.Context
	Example       *examples.Example
}

func (c *exampleContent) HTML() (h.HTML, error) {
	return c, fmt.Errorf("exampleContent.HTML is a dangerous primitive")
}

// Renders the example content including support for context sensitive
// text substitution.
func (c *exampleContent) Write(w io.Writer) (int, error) {
	e := c.Example
	wwwURL := fburl.URL{
		Env: c.Context.Env,
	}
	w = htmlwriter.New(w)
	tpl, err := template.New("example-" + e.URL).Parse(string(e.Content))
	if err != nil {
		// if template parsing fails, we ignore it. it's probably malformed html
		return w.Write(e.Content)
	}
	countingW := counting.NewWriter(w)
	err = tpl.Execute(countingW,
		struct {
			Rand     string // a random token
			RellFBNS string // the OG namespace
			RellURL  string // local http://www.fbrell.com/ URL
			WwwURL   string // server specific http://www.facebook.com/ URL
		}{
			Rand:     randString(10),
			RellFBNS: c.Context.AppNamespace,
			RellURL:  c.ContextParser.Default().AbsoluteURL("/").String(),
			WwwURL:   wwwURL.String(),
		})
	if err != nil {
		// if template execution fails, we ignore it. it's probably malformed html
		return w.Write(e.Content)
	}
	return countingW.Count(), err
}

// random string
func randString(length int) string {
	i := make([]byte, length)
	_, err := rand.Read(i)
	if err != nil {
		log.Panicf("failed to generate randString: %s", err)
	}
	return hex.EncodeToString(i)
}
