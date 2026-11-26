package main

// containsPoint reports whether (lat, lon) is inside the polygon of [lat, lon] pairs (ray casting).
func containsPoint(poly [][]float64, lat, lon float64) bool {
	inside := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		yi, xi := poly[i][0], poly[i][1]
		yj, xj := poly[j][0], poly[j][1]
		if (yi > lat) != (yj > lat) && lon < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}

// validPolygon requires >=4 in-range [lat, lon] pairs forming a closed ring.
func validPolygon(p [][]float64) bool {
	if len(p) < 4 {
		return false
	}
	for _, c := range p {
		if len(c) != 2 || c[0] < -90 || c[0] > 90 || c[1] < -180 || c[1] > 180 {
			return false
		}
	}
	return p[0][0] == p[len(p)-1][0] && p[0][1] == p[len(p)-1][1]
}
