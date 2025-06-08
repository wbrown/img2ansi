module github.com/wbrown/img2ansi/cmd/compute_fonts

go 1.22.5

require (
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0
	github.com/wbrown/img2ansi v0.0.0-00010101000000-000000000000
	golang.org/x/image v0.19.0
)

require gocv.io/x/gocv v0.37.0 // indirect

replace github.com/wbrown/img2ansi => ../..
