package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func petsCastsFixturePath() string { return filepath.Join("..", "..", fixtureDir, petsCastsFixture) }

// readPetsCasts splits the fixture into the elemental's lives and the images' summons.
func readPetsCasts(t *testing.T) (lives, summons []petsCastsRecord) {
	t.Helper()
	path := petsCastsFixturePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s; cut one from mod-sim-validation's TestPetfixFireElementalCasts and TestPetfixMirrorImageCasts", path)
	}
	records, err := readJSONLines[petsCastsRecord](path)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range records {
		if rec.isElemental() {
			lives = append(lives, rec)
		} else {
			summons = append(summons, rec)
		}
	}
	return lives, summons
}

// petsCastsImageSims hands out the sim's images for a lead, built once each.
func petsCastsImageSims(t *testing.T) func(lead time.Duration) map[bool]petsCastsImageRun {
	built := map[time.Duration]map[bool]petsCastsImageRun{}
	return func(lead time.Duration) map[bool]petsCastsImageRun {
		t.Helper()
		if _, ok := built[lead]; !ok {
			runs, err := simImageRuns(lead)
			if err != nil {
				t.Fatal(err)
			}
			built[lead] = runs
		}
		return built[lead]
	}
}

func petsCastsSummonLabel(n int, rec petsCastsRecord) string {
	if rec.Fighting {
		return fmt.Sprintf("summon %d, into a fight", n+1)
	}
	return fmt.Sprintf("summon %d, %v ahead of the pull", n+1, petsCastsLead(rec))
}

// TestPetsCastsRecords replays a live capture of the two scripted guardians' casts: the Greater Fire
// Elemental's lives against many of the sim's, and each Mirror Image against the sim's images summoned the
// same way with the same opener. Skips without the fixture.
func TestPetsCastsRecords(t *testing.T) {
	lives, summons := readPetsCasts(t)
	fighting := slices.ContainsFunc(summons, func(r petsCastsRecord) bool { return r.Fighting })
	ahead := slices.ContainsFunc(summons, func(r petsCastsRecord) bool { return !r.Fighting })
	if len(lives) == 0 || !fighting || !ahead {
		t.Fatalf("%d elemental lives, summons into a fight %v and ahead of one %v: the capture needs all three", len(lives), fighting, ahead)
	}
	checked := 0
	report := func(label string, checks []Check) {
		t.Helper()
		if len(checks) == 0 {
			t.Errorf("%s: nothing compared", label)
		}
		for _, check := range checks {
			checked++
			if !check.Passed {
				t.Errorf("%s: %s: %s", label, check.Name, check.Detail)
				continue
			}
			t.Logf("%s: %s: %s", label, check.Name, check.Detail)
		}
	}

	elemental, err := simElementalLives(petsCastsSimLives)
	if err != nil {
		t.Fatal(err)
	}
	report("Greater Fire Elemental", checkPetsCastsElemental(lives, elemental))

	images := petsCastsImageSims(t)
	for n, rec := range summons {
		lead := petsCastsLead(rec)
		report(petsCastsSummonLabel(n, rec), checkPetsCastsSummon(rec, images(lead), lead))
	}
	report("Mirror Images", []Check{checkPetsCastsOpeners(summons)})
	t.Logf("%d checks over %d lives and %d summons", checked, len(lives), len(summons))
}

// The replay has to fail a record the sim's guardian doesn't match: each case moves one recorded thing away.
func TestPetsCastsReplayCatchesAWrongRecord(t *testing.T) {
	lives, summons := readPetsCasts(t)
	elemental, err := simElementalLives(petsCastsSimLives)
	if err != nil {
		t.Fatal(err)
	}
	failed := func(t *testing.T, checks []Check, name string) {
		t.Helper()
		for _, check := range checks {
			if check.Name == name {
				if check.Passed {
					t.Errorf("%s still passes: %s", name, check.Detail)
				}
				return
			}
		}
		t.Errorf("no %s check", name)
	}

	for _, c := range []struct {
		name, check string
		move        func(*petsCastsRecord)
	}{
		{"Fire Blasts 2 s apart", "Fire Blast gaps", func(r *petsCastsRecord) {
			r.BlastGo = append(slices.Clone(r.BlastGo), r.BlastGo[len(r.BlastGo)-1]+2000)
		}},
		{"a Fire Nova landing a second after its start", "Fire Nova cast time", func(r *petsCastsRecord) {
			r.NovaGo = slices.Clone(r.NovaGo)
			r.NovaGo[0] -= 1000
		}},
		{"a Fire Nova broken by nothing", "broken Fire Novas", func(r *petsCastsRecord) {
			r.NovaFailed = append(slices.Clone(r.NovaFailed), r.BlastGo[0]+2000)
		}},
		{"a swing 1.5 s after the last", "swing waits", func(r *petsCastsRecord) {
			r.SwingWait = slices.Clone(r.SwingWait)
			r.SwingWait[1] = 1500
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			moved := slices.Clone(lives)
			c.move(&moved[0])
			failed(t, checkPetsCastsElemental(moved, elemental), c.check)
		})
	}
	// a dozen lives only tell a mean that's several swings off
	t.Run("five swings more a life", func(t *testing.T) {
		moved := slices.Clone(lives)
		for i := range moved {
			last := moved[i].SwingAt[len(moved[i].SwingAt)-1]
			moved[i].SwingAt = append(slices.Clone(moved[i].SwingAt), last, last, last, last, last)
		}
		failed(t, checkPetsCastsElemental(moved, elemental), "swings")
	})

	images := petsCastsImageSims(t)
	summon := func(fighting bool) petsCastsRecord {
		t.Helper()
		i := slices.IndexFunc(summons, func(r petsCastsRecord) bool { return r.Fighting == fighting })
		if i < 0 {
			t.Fatalf("no summon with fighting %v", fighting)
		}
		rec := summons[i]
		rec.Images = slices.Clone(rec.Images)
		return rec
	}
	for _, c := range []struct {
		name, check string
		fighting    bool
		move        func(*petsCastsImage)
	}{
		{"an image into a fight opening on a Fire Blast", "image 1 opener", true, func(img *petsCastsImage) {
			img.Opener = petsCastsBlastOpener
		}},
		{"an image without its second Fire Blast", "image 1 Fire Blasts", true, func(img *petsCastsImage) {
			img.FireBlastGo = slices.Delete(slices.Clone(img.FireBlastGo), 1, 2)
		}},
		{"an image two Frostbolts short", "image 1 Frostbolts", true, func(img *petsCastsImage) {
			img.FrostboltGo = img.FrostboltGo[:len(img.FrostboltGo)-2]
		}},
		{"an image waiting a second after the pull", "image 1 Frostbolts", false, func(img *petsCastsImage) {
			img.FrostboltGo = slices.Clone(img.FrostboltGo)
			for i := range img.FrostboltGo {
				img.FrostboltGo[i] += 1000
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := summon(c.fighting)
			c.move(&rec.Images[0])
			lead := petsCastsLead(rec)
			failed(t, checkPetsCastsSummon(rec, images(lead), lead), c.check)
		})
	}
	t.Run("two images of three", func(t *testing.T) {
		rec := summon(true)
		rec.Images = rec.Images[:2]
		failed(t, checkPetsCastsSummon(rec, images(petsCastsLead(rec)), petsCastsLead(rec)), "images")
	})
	t.Run("every image opening on a Frostbolt", func(t *testing.T) {
		var moved []petsCastsRecord
		// many summons, so the even pick shows
		for range 4 {
			for _, rec := range summons {
				rec.Images = slices.Clone(rec.Images)
				for i := range rec.Images {
					rec.Images[i].Opener = "Frostbolt"
				}
				moved = append(moved, rec)
			}
		}
		if check := checkPetsCastsOpeners(moved); check.Passed {
			t.Errorf("%s still passes: %s", check.Name, check.Detail)
		}
	})
}
