# G502 HERO / SE model assets

These assets were supplied for future G502 support and are intentionally not
loaded by the application yet.

## Contents

- `model_0.obj`: 4,417 vertices and 10,981 faces.
- `model_1.obj`: 8,221 vertices and 16,833 faces.
- `material-001-*`: base color, metallic, normal, and roughness textures.
- `material-002-*`: base color, emissive, metallic, normal, and roughness
  textures.

The OBJ geometry is preserved unchanged. Textures were resized from 2048 px to
1024 px and converted to WebP so the staged asset set is about 3.5 MB instead
of roughly 36 MB.

## Material caveat

Both OBJ files reference `model_0.mtl` or `model_1.mtl`, but those material
files were not present in the supplied folder. The OBJ files also contain no
`usemtl` directives. The matching numbers suggest that `model_0.obj` belongs
with Material 001 and `model_1.obj` with Material 002, but that assignment must
be visually verified before the model is integrated or converted to GLB.

Confirm that the model and textures may be redistributed before including them
in a public release.
