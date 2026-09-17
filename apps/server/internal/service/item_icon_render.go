package service

import (
	"image"
	"image/color"
	"math"
	"sort"
)

type iconVec struct{ x, y, z float64 }

func rotateIcon(v iconVec, axis int, angle float64) iconVec {
	s, c := math.Sincos(angle * math.Pi / 180)
	switch axis {
	case 0:
		return iconVec{v.x, v.y*c - v.z*s, v.y*s + v.z*c}
	case 1:
		return iconVec{v.x*c + v.z*s, v.y, -v.x*s + v.z*c}
	default:
		return iconVec{v.x*c - v.y*s, v.x*s + v.y*c, v.z}
	}
}
func renderIsometricCube(tex image.Image) image.Image {
	r := newIconResources()
	m, _ := r.model("minecraft:block/cube_all", map[string]bool{})
	img, _ := renderBlockModel(m, func(string) image.Image { return tex })
	return img
}

// Render actual element geometry, UVs and inherited GUI transforms. Model Y is
// upward; screen Y is downward. A depth buffer resolves overlapping elements.
func renderBlockModel(m iconModel, texture func(string) image.Image) (image.Image, string) {
	const size = 32
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	transform := m.Display["gui"]
	scale := [3]float64{1, 1, 1}
	if transform.Scale != nil {
		scale = *transform.Scale
	}
	type face struct {
		p            [4]iconVec
		uv           [4][2]float64
		tex          image.Image
		light, depth float64
	}
	faces := []face{}
	for _, el := range m.Elements {
		a, b := el.From, el.To
		for _, dir := range []string{"down", "up", "north", "south", "west", "east"} {
			f, ok := el.Faces[dir]
			if !ok {
				continue
			}
			tex := texture(f.Texture)
			if tex == nil {
				return nil, "missing_texture"
			}
			// Tint handlers live in game code. Do not present an untinted image as exact.
			if f.TintIndex != nil && *f.TintIndex >= 0 {
				return nil, "requires_tint"
			}
			var p [4]iconVec
			var uv []float64
			switch dir {
			case "up":
				p = [4]iconVec{{a[0], b[1], a[2]}, {b[0], b[1], a[2]}, {b[0], b[1], b[2]}, {a[0], b[1], b[2]}}
				uv = []float64{a[0], a[2], b[0], b[2]}
			case "down":
				p = [4]iconVec{{a[0], a[1], b[2]}, {b[0], a[1], b[2]}, {b[0], a[1], a[2]}, {a[0], a[1], a[2]}}
				uv = []float64{a[0], 16 - b[2], b[0], 16 - a[2]}
			case "north":
				p = [4]iconVec{{b[0], b[1], a[2]}, {a[0], b[1], a[2]}, {a[0], a[1], a[2]}, {b[0], a[1], a[2]}}
				uv = []float64{16 - b[0], 16 - b[1], 16 - a[0], 16 - a[1]}
			case "south":
				p = [4]iconVec{{a[0], b[1], b[2]}, {b[0], b[1], b[2]}, {b[0], a[1], b[2]}, {a[0], a[1], b[2]}}
				uv = []float64{a[0], 16 - b[1], b[0], 16 - a[1]}
			case "west":
				p = [4]iconVec{{a[0], b[1], a[2]}, {a[0], b[1], b[2]}, {a[0], a[1], b[2]}, {a[0], a[1], a[2]}}
				uv = []float64{a[2], 16 - b[1], b[2], 16 - a[1]}
			case "east":
				p = [4]iconVec{{b[0], b[1], b[2]}, {b[0], b[1], a[2]}, {b[0], a[1], a[2]}, {b[0], a[1], b[2]}}
				uv = []float64{16 - b[2], 16 - b[1], 16 - a[2], 16 - a[1]}
			}
			if len(f.UV) == 4 {
				uv = f.UV
			}
			for i, v := range p {
				if rot := el.Rotation; rot != nil {
					o := rot.Origin
					v = iconVec{v.x - o[0], v.y - o[1], v.z - o[2]}
					axis := 0
					if rot.Axis == "y" {
						axis = 1
					}
					if rot.Axis == "z" {
						axis = 2
					}
					v = rotateIcon(v, axis, rot.Angle)
					if rot.Rescale {
						c := math.Cos(rot.Angle * math.Pi / 180)
						if math.Abs(c) < .01 {
							return nil, "unsupported_rotation"
						}
						s := 1 / c
						if axis != 0 {
							v.x *= s
						}
						if axis != 1 {
							v.y *= s
						}
						if axis != 2 {
							v.z *= s
						}
					}
					v = iconVec{v.x + o[0], v.y + o[1], v.z + o[2]}
				}
				v = iconVec{(v.x - 8) * scale[0], (v.y - 8) * scale[1], (v.z - 8) * scale[2]}
				// Minecraft's quaternion rotationXYZ applies Z, then Y, then X.
				for axis := 2; axis >= 0; axis-- {
					v = rotateIcon(v, axis, transform.Rotation[axis])
				}
				p[i] = iconVec{16 + 2*(v.x+transform.Translation[0]), 16 - 2*(v.y+transform.Translation[1]), v.z + transform.Translation[2]}
			}
			// Clockwise geometry in model space becomes counterclockwise on screen
			// for front-facing surfaces (after the screen-Y inversion).
			cross := (p[1].x-p[0].x)*(p[3].y-p[0].y) - (p[1].y-p[0].y)*(p[3].x-p[0].x)
			if cross <= 1e-8 {
				continue
			}
			light := .8
			if dir == "up" {
				light = 1
			}
			if dir == "down" {
				light = .5
			}
			if dir == "east" || dir == "west" {
				light = .65
			}
			if m.GUILight == "front" || (el.Shade != nil && !*el.Shade) {
				light = 1
			}
			coords := [4][2]float64{{uv[0], uv[1]}, {uv[2], uv[1]}, {uv[2], uv[3]}, {uv[0], uv[3]}}
			rotated := coords
			for i := 0; i < 4; i++ {
				rotated[i] = coords[(i+((f.Rotation/90)%4+4)%4)%4]
			}
			faces = append(faces, face{p: p, uv: rotated, tex: tex, light: light, depth: (p[0].z + p[1].z + p[2].z + p[3].z) / 4})
		}
	}
	sort.SliceStable(faces, func(i, j int) bool { return faces[i].depth < faces[j].depth })
	depth := make([]float64, size*size)
	for i := range depth {
		depth[i] = math.Inf(-1)
	}
	for _, f := range faces {
		A, B, D := f.p[0], f.p[1], f.p[3]
		ex, ey := B.x-A.x, B.y-A.y
		fx, fy := D.x-A.x, D.y-A.y
		det := ex*fy - ey*fx
		bounds := f.tex.Bounds()
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx, dy := float64(x)+.5-A.x, float64(y)+.5-A.y
				u, v := (dx*fy-dy*fx)/det, (ex*dy-ey*dx)/det
				if u < 0 || v < 0 || u > 1 || v > 1 {
					continue
				}
				z := A.z + u*(B.z-A.z) + v*(D.z-A.z)
				if z < depth[y*size+x]-1e-6 {
					continue
				}
				tu := f.uv[0][0] + u*(f.uv[1][0]-f.uv[0][0]) + v*(f.uv[3][0]-f.uv[0][0])
				tv := f.uv[0][1] + u*(f.uv[1][1]-f.uv[0][1]) + v*(f.uv[3][1]-f.uv[0][1])
				sx := min(bounds.Dx()-1, max(0, int(tu/16*float64(bounds.Dx()))))
				sy := min(bounds.Dy()-1, max(0, int(tv/16*float64(bounds.Dy()))))
				c := color.NRGBAModel.Convert(f.tex.At(bounds.Min.X+sx, bounds.Min.Y+sy)).(color.NRGBA)
				if c.A == 0 {
					continue
				}
				c.R = uint8(float64(c.R) * f.light)
				c.G = uint8(float64(c.G) * f.light)
				c.B = uint8(float64(c.B) * f.light)
				dst := out.NRGBAAt(x, y)
				alpha := float64(c.A) / 255
				da := float64(dst.A) / 255
				oa := alpha + da*(1-alpha)
				blend := func(a, b uint8) uint8 { return uint8((float64(a)*alpha + float64(b)*da*(1-alpha)) / oa) }
				out.SetNRGBA(x, y, color.NRGBA{blend(c.R, dst.R), blend(c.G, dst.G), blend(c.B, dst.B), uint8(oa * 255)})
				depth[y*size+x] = z
			}
		}
	}
	if len(faces) == 0 {
		return nil, "unsupported_model"
	}
	return out, "elements"
}
