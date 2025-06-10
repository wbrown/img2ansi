module github.com/wbrown/img2ansi/cmd/compute_glyphs

go 1.23.0

toolchain go1.24.2

replace github.com/wbrown/img2ansi => ../..

require (
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0
	github.com/wbrown/img2ansi v0.0.0
	golang.org/x/image v0.28.0
)

require gocv.io/x/gocv v0.37.0 // indirect
