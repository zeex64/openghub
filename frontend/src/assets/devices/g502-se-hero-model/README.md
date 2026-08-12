# G502 HERO / SE model assets

These assets supply the interactive G502 HERO view on the Home page.

## Contents

- `model_0.obj`: 4,417 vertices and 10,981 faces.
- `model_1.obj`: 8,221 vertices and 16,833 faces.
- `material-001-*`: base color, metallic, normal, and roughness textures.
- `material-002-*`: base color, emissive, metallic, normal, and roughness
  textures.

The OBJ geometry is preserved unchanged. Textures were resized from 2048 px to
1024 px and converted to WebP so the staged asset set is about 3.5 MB instead
of roughly 36 MB.

## Material assignment

Both OBJ files reference `model_0.mtl` or `model_1.mtl`, but those material
files were not present in the supplied folder. The viewer therefore restores
the association from the UV data: Material 002 belongs to `model_0.obj` (its
UVs cover the Logitech logo and DPI-indicator emissive pixels), while Material
001 belongs to `model_1.obj`.

Confirm that the model and textures may be redistributed before including them
in a public release.
