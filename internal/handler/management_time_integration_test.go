package handler

import (
	"fmt"
	"testing"
	"time"
)

// Reproduces a stored timestamp slightly ahead of the application clock. This
// used to reject a valid brand/product publication with HTTP 400 "time".
func TestPostgresHTTPManagementKeepsMetadataTimeMonotonic(t *testing.T) {
	h, db := newPVMHTTP(t)
	owner, uid := pvmAccount(t, h, "clock_owner")
	editor, editorID := pvmAccount(t, h, "clock_editor")
	b := pvmDecode[brandResponse](t, owner.call("POST", "/brands", `{"name":"Clock Brand","slug":"clock-brand"}`, 201))
	var before time.Time
	err := db.QueryRow(`UPDATE brands SET description='Clock fixture',logo_url='https://example.com/logo.jpg',updated_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING updated_at`, b.ID).Scan(&before)
	if err != nil {
		t.Fatal(err)
	}
	b = pvmDecode[brandResponse](t, owner.call("POST", fmt.Sprintf("/brands/%d/publish", b.ID), `{}`, 200))
	if b.UpdatedAt.Before(before) {
		t.Fatal("brand metadata moved backwards")
	}
	p := pvmDecode[productResponse](t, owner.call("POST", "/products", fmt.Sprintf(`{"brand_id":%d,"name":"Clock Hoodie","description":"Complete hoodie","price_kopecks":100000,"category":"hoodie","materials":["cotton"]}`, b.ID), 201))
	path := fmt.Sprintf("/products/%d", p.ID)
	c := pvmDecode[managedColorResponse](t, owner.call("POST", path+"/colors", `{"color":"Black"}`, 201))
	pvmLegacyImage(t, h, uid, p.ID, c.ID, "https://example.com/front.jpg", 0)
	owner.call("POST", fmt.Sprintf("%s/colors/%d/variants", path, c.ID), `{"size":"M","stock":1,"sku":""}`, 201)
	err = db.QueryRow(`UPDATE products SET updated_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING updated_at`, p.ID).Scan(&before)
	if err != nil {
		t.Fatal(err)
	}
	p = pvmDecode[productResponse](t, owner.call("POST", path+"/publish", `{}`, 200))
	if p.UpdatedAt.Before(before) {
		t.Fatal("product metadata moved backwards")
	}
	bpath := fmt.Sprintf("/brands/%d", b.ID)
	owner.call("POST", bpath+"/invitations", `{"username":"clock_editor"}`, 201)
	if _, err = db.Exec(`UPDATE brand_memberships SET updated_at=clock_timestamp()+interval '2 seconds' WHERE brand_id=$1 AND user_id=$2`, b.ID, editorID); err != nil {
		t.Fatal(err)
	}
	editor.call("POST", bpath+"/invitations/accept", `{}`, 200)
	owner.call("DELETE", fmt.Sprintf("%s/members/%d", bpath, editorID), "", 204)
}
