img2ansi
========
Image to ANSI conversion:

Major features:
* Modified Atkinson dithering
* Edge detection
* ANSI compression
* Subcharacter block rendering with 2 colors per character.
* Separate foreground and background palette for terminals that support it.
* Color quantization
* Maximum file size targeting
* For IRC: line limits

Requires OpenCV 4 to be installed.

```
  -edge int
    	Color difference threshold for edge detection skipping (default 100)
  -input string
    	Path to the input image file (required)
  -maxchars int
    	Maximum number of characters in the output (default 1048576)
  -maxline int
    	Maximum number of characters in a line, 0 for no limit
  -output string
    	Path to save the output (if not specified, prints to stdout)
  -quantization int
    	Quantization factor (default 256)
  -scale float
    	Scale factor for the output image (default 3)
  -separate
    	Use separate palettes for foreground and background colors
  -shading
    	Enable shading for more detailed output
  -space
    	Convert block characters to spaces
  -width int
    	Target width of the output image (default 100)
```
