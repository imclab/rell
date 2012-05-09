package context

import (
	"github.com/nshah/go.subset"
	"net/url"
	"testing"
)

func TestDefaultContext(t *testing.T) {
	t.Parallel()
	context := FromValues(url.Values{})
	subset.Assert(t, defaultContext, context)
}

func TestCustomAppID(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Add("appid", "123")
	context := FromValues(values)
	if context.AppID != 123 {
		t.Fatalf("Did not find expected app id 123 instead found %d", context.AppID)
	}
}

func TestCustomStatus(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Add("status", "0")
	context := FromValues(values)
	if context.Status {
		t.Fatal("Was expecting status to be false.")
	}
}

func TestComplex(t *testing.T) {
	t.Parallel()
	values := url.Values{}
	values.Add("status", "1")
	values.Add("server", "beta")
	values.Add("locale", "en_PI")
	values.Add("version", "old")
	values.Add("channel", "false")
	expected := &Context{
		Status:     true,
		Env:        "beta",
		Locale:     "en_PI",
		Version:    Old,
		UseChannel: false,
	}
	context := FromValues(values)
	subset.Assert(t, expected, context)
}

func TestPageTabURLBeta(t *testing.T) {
	t.Parallel()
	expected := "http://www.beta.facebook.com/pages/" +
		"Rell-Page-for-Tabs/141929622497380?sk=app_184484190795&app_data=beta"
	values := url.Values{}
	values.Add("server", "beta")
	context := FromValues(values)
	actual := context.PageTabURL()
	if actual != expected {
		t.Fatalf("Did not find expected URL %s instead found %s", expected, actual)
	}
}

func TestPageTabURL(t *testing.T) {
	t.Parallel()
	expected := "http://www.facebook.com/pages/Rell-Page-for-Tabs" +
		"/141929622497380?sk=app_184484190795"
	context := FromValues(url.Values{})
	if context.PageTabURL() != expected {
		t.Fatalf("Did not find expected URL %s instead found %s",
			expected, context.PageTabURL())
	}
}

func TestCanvasURLBeta(t *testing.T) {
	t.Parallel()
	expected := "http://apps.beta.facebook.com/fbrelll/?server=beta"
	values := url.Values{}
	values.Add("server", "beta")
	context := FromValues(values)
	if context.CanvasURL() != expected {
		t.Fatalf("Did not find expected URL %s instead found %s",
			expected, context.CanvasURL())
	}
}

func TestCanvasURL(t *testing.T) {
	t.Parallel()
	expected := "http://apps.facebook.com/fbrelll/"
	context := FromValues(url.Values{})
	if context.CanvasURL() != expected {
		t.Fatalf("Did not find expected URL %s instead found %s",
			expected, context.CanvasURL())
	}
}