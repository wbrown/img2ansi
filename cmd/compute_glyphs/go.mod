module github.com/wbrown/img2ansi/cmd/compute_glyphs

go 1.24.0

toolchain go1.24.7

replace github.com/wbrown/img2ansi => ../..

require (
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0
	github.com/wbrown/img2ansi v0.0.0-00010101000000-000000000000
	golang.org/x/image v0.34.0
)
