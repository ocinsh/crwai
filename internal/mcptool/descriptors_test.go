package mcptool

import (
	"reflect"
	"strings"
	"testing"
)

// tools pairs every descriptor with the In struct whose fields it documents. A
// new tool must be added here, which is the point: the table is what keeps the
// prose an agent reads and the schema the SDK generates from drifting apart.
var tools = []struct {
	descriptor ToolDescriptor
	in         any
}{
	{ListSignatures, ListSignaturesIn{}},
	{GetFunctionBody, GetFunctionBodyIn{}},
	{GetFunction, GetFunctionIn{}},
	{ReadInterface, ReadInterfaceIn{}},
	{ReadStruct, ReadStructIn{}},
	{WriteFunction, WriteFunctionIn{}},
	{OutlineMarkdown, OutlineMarkdownIn{}},
	{ReadSection, ReadSectionIn{}},
	{WriteSection, WriteSectionIn{}},
	{ListRequests, ListRequestsIn{}},
	{ReadRequest, ReadRequestIn{}},
}

// TestParamsMatchTheInputStruct is the guard on the rule that a descriptor's
// Params mirror the fields of its In struct. A parameter documented but absent
// from the struct is a promise the schema does not keep; a field present but
// undocumented is a parameter the agent is never told about.
func TestParamsMatchTheInputStruct(t *testing.T) {
	for _, tool := range tools {
		t.Run(tool.descriptor.Name, func(t *testing.T) {
			fields := map[string]bool{}
			typ := reflect.TypeOf(tool.in)
			for i := 0; i < typ.NumField(); i++ {
				name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
				if name != "" && name != "-" {
					fields[name] = true
				}
			}

			documented := map[string]bool{}
			for _, p := range tool.descriptor.Params {
				if !fields[p.Name] {
					t.Errorf("parameter %q is documented but is not a field of %T", p.Name, tool.in)
				}
				documented[p.Name] = true
			}
			for name := range fields {
				if !documented[name] {
					t.Errorf("field %q of %T is not documented in Params", name, tool.in)
				}
			}
		})
	}
}

// TestEveryToolIsDescribed checks the descriptors carry the metadata an agent
// steers on. A tool with no mission is a tool the model will pick by name alone.
func TestEveryToolIsDescribed(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range tools {
		d := tool.descriptor
		if d.Name == "" {
			t.Fatal("a descriptor has no name")
		}
		if seen[d.Name] {
			t.Errorf("tool name %q is registered twice", d.Name)
		}
		seen[d.Name] = true
		if len(d.Mission) < 40 {
			t.Errorf("%s: mission is too thin to steer tool selection: %q", d.Name, d.Mission)
		}
		if len(d.Examples) == 0 {
			t.Errorf("%s: no example", d.Name)
		}
		for _, ex := range d.Examples {
			if !strings.HasPrefix(ex, d.Name+"{") {
				t.Errorf("%s: example does not call the tool it documents: %q", d.Name, ex)
			}
		}
	}
}

// TestRenderFoldsTheDescriptor pins the shape of the description string the SDK
// sends on the wire: the mission first, then the parameters, then the examples.
func TestRenderFoldsTheDescriptor(t *testing.T) {
	got := ToolDescriptor{
		Name:     "demo",
		Mission:  "Do the thing.",
		Params:   []ParamDoc{{Name: "path", Description: "where"}},
		Examples: []string{`demo{"path":"a.go"}`},
	}.render()

	for _, want := range []string{"Do the thing.", "Parameters:", "- path: where", "Examples:", `demo{"path":"a.go"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered description is missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "Parameters:") > strings.Index(got, "Examples:") {
		t.Error("parameters are rendered after the examples")
	}
}
