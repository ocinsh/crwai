module github.com/ocinsh/crwai

go 1.26

// NOTE: This is a source-only skeleton. The two dependencies below are declared
// so the import paths in the code are stable, but `go mod tidy` has NOT been run
// and the grammar packages (e.g. github.com/tree-sitter/tree-sitter-go/bindings/go)
// are deferred until languages are implemented. Resolving these requires a C
// toolchain (CGO) — do not set CGO_ENABLED=0.
//
// require (
// 	github.com/modelcontextprotocol/go-sdk latest
// 	github.com/tree-sitter/go-tree-sitter  latest
// )

require (
	github.com/UserNobody14/tree-sitter-dart v0.0.0-20260520003023-a9bdfa3db2fb
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/modelcontextprotocol/go-sdk v1.6.1
	github.com/spf13/cobra v1.10.2
	github.com/tree-sitter/go-tree-sitter v0.25.0
	github.com/tree-sitter/tree-sitter-c v0.23.4
	github.com/tree-sitter/tree-sitter-cpp v0.23.4
	github.com/tree-sitter/tree-sitter-go v0.25.0
	github.com/tree-sitter/tree-sitter-java v0.23.5
	github.com/tree-sitter/tree-sitter-javascript v0.23.1
	github.com/tree-sitter/tree-sitter-python v0.25.0
	github.com/tree-sitter/tree-sitter-rust v0.24.0
	github.com/tree-sitter/tree-sitter-typescript v0.23.2
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc // indirect
	github.com/charmbracelet/x/ansi v0.8.0 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13-0.20250311204145-2c3ea96c31dd // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)
