package main

import "testing"

func TestContainsPoint(t *testing.T) {
	polygon := [][]float64{{12, 77}, {12, 78}, {13, 78}, {13, 77}, {12, 77}}
	if !containsPoint(polygon, 12.5, 77.5) {
		t.Fatal("expected point inside polygon")
	}
	if containsPoint(polygon, 14, 77.5) {
		t.Fatal("expected point outside polygon")
	}
}

func TestValidPolygonRejectsMalformedCoordinates(t *testing.T) {
	if validPolygon([][]float64{{1}, {2, 2}, {3, 3}, {1}}) {
		t.Fatal("accepted malformed coordinate pair")
	}
	if validPolygon([][]float64{{1, 1}, {1, 2}, {2, 2}, {1, 1.1}}) {
		t.Fatal("accepted an open polygon")
	}
}
