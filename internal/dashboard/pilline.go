package dashboard

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Pillow's ImageDraw.line for width > 1 (no joints below width 5), ported from libImaging/Draw.c
// (ImagingDrawWideLine + polygon_generic, non-alpha path) so traces match dashboard.py exactly.

func roundUp(f float64) int {
	if f >= 0 {
		return int(math.Floor(f + 0.5))
	}
	return -int(math.Floor(math.Abs(f) + 0.5))
}

func roundDown(f float64) int {
	if f >= 0 {
		return int(math.Ceil(f - 0.5))
	}
	return -int(math.Ceil(math.Abs(f) - 0.5))
}

type edge struct {
	d                      int
	x0, y0                 int
	xmin, ymin, xmax, ymax int
	dx                     float32
}

func newEdge(x0, y0, x1, y1 int) edge {
	e := edge{x0: x0, y0: y0}
	e.xmin, e.xmax = min(x0, x1), max(x0, x1)
	e.ymin, e.ymax = min(y0, y1), max(y0, y1)
	if y0 == y1 {
		e.d, e.dx = 0, 0
	} else {
		e.dx = float32(x1-x0) / float32(y1-y0)
		if y0 == e.ymin {
			e.d = 1
		} else {
			e.d = -1
		}
	}
	return e
}

func hline(dst *image.RGBA, x0, y, x1 int, c color.RGBA) {
	b := dst.Bounds()
	if y < 0 || y >= b.Dy() {
		return
	}
	if x0 < 0 {
		x0 = 0
	} else if x0 >= b.Dx() {
		return
	}
	if x1 < 0 {
		return
	} else if x1 >= b.Dx() {
		x1 = b.Dx() - 1
	}
	for x := x0; x <= x1; x++ {
		o := dst.PixOffset(x, y)
		dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = c.R, c.G, c.B, 255
	}
}

func roundf(v float32) float32 { return float32(math.Round(float64(v))) }

func polygon(dst *image.RGBA, es []edge, c color.RGBA) {
	ysize := dst.Bounds().Dy()
	ymin, ymax := ysize-1, 0
	var table []*edge
	for i := range es {
		e := &es[i]
		ymin = min(ymin, e.ymin)
		ymax = max(ymax, e.ymax)
		if e.ymin == e.ymax {
			hline(dst, e.xmin, e.ymin, e.xmax, c)
			continue
		}
		table = append(table, e)
	}
	ymin = max(ymin, 0)
	ymax = min(ymax, ysize)
	xx := make([]float32, 0, len(table)*2)
	for ; ymin <= ymax; ymin++ {
		xx = xx[:0]
		for i, cur := range table {
			if ymin < cur.ymin || ymin > cur.ymax {
				continue
			}
			xx = append(xx, float32(ymin-cur.y0)*cur.dx+float32(cur.x0))
			j := len(xx)
			if ymin == cur.ymax && ymin < ymax {
				xx = append(xx, xx[j-1])
			} else if (ymin == cur.ymin || ymin == cur.ymax) && cur.dx != 0 {
				for k := 0; k < i; k++ {
					o := table[k]
					if (ymin != o.ymin && ymin != o.ymax) || o.dx == 0 {
						continue
					}
					if roundf(xx[j-1]) == roundf(float32(ymin-o.y0)*o.dx+float32(o.x0)) {
						off := 1
						if ymin == cur.ymax {
							off = -1
						}
						adj := float32(ymin+off-cur.y0)*cur.dx + float32(cur.x0)
						if ymin+off >= o.ymin && ymin+off <= o.ymax {
							adjO := float32(ymin+off-o.y0)*o.dx + float32(o.x0)
							if xx[j-1] > adj+1 && xx[j-1] > adjO+1 {
								xx[j-1] = roundf(float32(math.Max(float64(adj), float64(adjO)))) + 1
							} else if xx[j-1] < adj-1 && xx[j-1] < adjO-1 {
								xx[j-1] = roundf(float32(math.Min(float64(adj), float64(adjO)))) - 1
							}
							break
						}
					}
				}
			}
		}
		sort.Slice(xx, func(a, b int) bool { return xx[a] < xx[b] })
		for i := 1; i < len(xx); i += 2 {
			hline(dst, roundUp(float64(xx[i-1])), ymin, roundDown(float64(xx[i])), c)
		}
	}
}

// wideLine is ImagingDrawWideLine.
func wideLine(dst *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, width int) {
	dx, dy := x1-x0, y1-y0
	if dx == 0 && dy == 0 {
		if x0 >= 0 && y0 >= 0 && x0 < dst.Bounds().Dx() && y0 < dst.Bounds().Dy() {
			dst.SetRGBA(x0, y0, c)
		}
		return
	}
	big := math.Hypot(float64(dx), float64(dy))
	small := float64(width-1) / 2
	rmax := float64(roundUp(small)) / big
	rmin := float64(roundDown(small)) / big
	dxmin, dxmax := roundDown(rmin*float64(dy)), roundDown(rmax*float64(dy))
	dymin, dymax := roundDown(rmin*float64(dx)), roundDown(rmax*float64(dx))
	v := [4][2]int{{x0 - dxmin, y0 + dymax}, {x1 - dxmin, y1 + dymax}, {x1 + dxmax, y1 - dymin}, {x0 + dxmax, y0 - dymin}}
	es := []edge{
		newEdge(v[0][0], v[0][1], v[1][0], v[1][1]),
		newEdge(v[1][0], v[1][1], v[2][0], v[2][1]),
		newEdge(v[2][0], v[2][1], v[3][0], v[3][1]),
		newEdge(v[3][0], v[3][1], v[0][0], v[0][1]),
	}
	polygon(dst, es, c)
}

// polyline is ImageDraw.line(points, width=w) for w > 1 (C truncates coordinates to int).
func polyline(dst *image.RGBA, pts [][2]float64, c color.RGBA, width int) {
	for i := 0; i+1 < len(pts); i++ {
		wideLine(dst, int(pts[i][0]), int(pts[i][1]), int(pts[i+1][0]), int(pts[i+1][1]), c, width)
	}
}
