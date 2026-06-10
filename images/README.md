# Test Image Corpus

Reference photos for the quality harness (`diffusion_quality_test.go`):
`TestDiffusionQualityPhotos`, `TestConverterArms`, and
`TestBlockAlphabetQuality` run against every PNG in this directory, and
the standings tables in `docs/glyph-research/README.md` are measured on
it. Carried over from the `feature/font-png-rendering` research branch.

- `mandrill.png` — the classic dithering/compression test image
  (USC-SIPI image database, 4.2.03)
- `fox.png` — photographic test: fur texture, snow, shallow depth of field
- `wheel.png` — color wheel: flat saturated wedges and hue boundaries

Adding a PNG here automatically adds it to every photo-driven harness
run. Keep additions small and justified — each one slows the suite.
