package main

import (
	"fmt"
	"testing"
)

func TestListEndpointsPaginateWithTotalCount(t *testing.T) {
	a := newApp("")
	for i := 0; i < 5; i++ {
		call(t, a, "POST", "/geofences", resp{"name": fmt.Sprint("z", i), "coordinates": square, "category": "toll_zone"})
		call(t, a, "POST", "/vehicles", resp{"vehicle_number": fmt.Sprint("V", i), "driver_name": "d", "vehicle_type": "t", "phone": "1"})
	}
	_, f, _ := call(t, a, "POST", "/geofences", resp{"name": "last", "coordinates": square, "category": "toll_zone"})
	call(t, a, "POST", "/alerts/configure", resp{"geofence_id": f["id"], "event_type": "both"})

	for key, path := range map[string]string{"geofences": "/geofences", "vehicles": "/vehicles"} {
		_, all, _ := call(t, a, "GET", path, nil) // no params: everything, as before
		_, page, _ := call(t, a, "GET", path+"?limit=2&offset=1", nil)
		if len(page[key].([]any)) != 2 || page["total_count"].(float64) != float64(len(all[key].([]any))) {
			t.Fatalf("%s pagination: %v", path, page)
		}
		_, tail, _ := call(t, a, "GET", path+"?limit=50&offset=4", nil)
		if n := len(tail[key].([]any)); n != len(all[key].([]any))-4 {
			t.Fatalf("%s offset tail: %d", path, n)
		}
	}
	_, al, _ := call(t, a, "GET", "/alerts?limit=1", nil)
	if len(al["alerts"].([]any)) != 1 || al["total_count"].(float64) != 1 {
		t.Fatalf("alerts pagination: %v", al)
	}
}

func TestDeleteAlertRuleStopsAlerting(t *testing.T) {
	a := newApp("")
	_, veh := seed(t, a)
	_, list, _ := call(t, a, "GET", "/alerts", nil)
	id := list["alerts"].([]any)[0].(map[string]any)["alert_id"].(string)

	if code, out, _ := call(t, a, "DELETE", "/alerts/"+id, nil); code != 200 || out["status"] != "deleted" || out["time_ns"] == nil {
		t.Fatalf("delete: %d %v", code, out)
	}
	if code, _, _ := call(t, a, "DELETE", "/alerts/"+id, nil); code != 404 {
		t.Fatalf("second delete should be 404, got %d", code)
	}
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	if n := len(a.state.Events); n != 0 {
		t.Fatalf("deleted rule still fired %d event(s)", n)
	}
	_, list, _ = call(t, a, "GET", "/alerts", nil)
	if len(list["alerts"].([]any)) != 0 {
		t.Fatal("rule still listed")
	}
}

func TestDeletedRuleStaysDeletedAndSyncsToPeers(t *testing.T) {
	a, url := pgApp(t)
	b := secondApp(t, url)
	seed(t, a)
	eventually(t, "B sees the rule", func() bool { b.mu.RLock(); defer b.mu.RUnlock(); return len(b.state.Rules) == 1 })
	_, list, _ := call(t, a, "GET", "/alerts", nil)
	id := list["alerts"].([]any)[0].(map[string]any)["alert_id"].(string)
	call(t, a, "DELETE", "/alerts/"+id, nil)
	eventually(t, "B drops the rule", func() bool { b.mu.RLock(); defer b.mu.RUnlock(); return len(b.state.Rules) == 0 })
	a.db.close()
	c := newApp("")
	if err := c.useDB(t.Context(), url); err != nil {
		t.Fatal(err)
	}
	a.db = c.db
	if len(c.state.Rules) != 0 {
		t.Fatal("deleted rule came back after restart")
	}
}
