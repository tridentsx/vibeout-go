package game

import "testing"

// Retail's main screen is three columns -- RACE TYPE, TEAM, CLASS AND TRACK -- then
// START and OPTIONS, so the cursor has five stops rather than a six-item list.
func TestMainScreenHasFiveCursorStops(t *testing.T) {
	c := NewMenuCursor()
	ctx := &GameContext{}
	if mainScreenPositions != 5 {
		t.Fatalf("mainScreenPositions is %d, want 5", mainScreenPositions)
	}
	if got := c.itemCount(ctx); got != mainScreenPositions {
		t.Fatalf("main screen has %d cursor stops, want %d", got, mainScreenPositions)
	}
	// Each column opens its screen.
	for stop, want := range map[int]MenuScreenID{0: ScreenRaceType, 1: ScreenTeam, 2: ScreenTrack, 4: ScreenOptions} {
		c := NewMenuCursor()
		c.Move(stop, ctx)
		if act := c.Activate(ctx); act != ActionNone {
			t.Errorf("stop %d returned action %v, want navigation", stop, act)
		}
		if c.Screen() != want {
			t.Errorf("stop %d opened %v, want %v", stop, c.Screen(), want)
		}
	}
}

// Classes are VECTOR/VENOM/RAPIER/PHANTOM, matching the SpeedClass constants by
// index so selecting a row can assign it directly.
func TestClassScreenMatchesSpeedClassOrder(t *testing.T) {
	want := []string{"VECTOR", "VENOM", "RAPIER", "PHANTOM"}
	items := Screens[ScreenClass].Items
	if len(items) != len(want) {
		t.Fatalf("%d classes, want 4", len(items))
	}
	for i := range want {
		if items[i].Label != want[i] {
			t.Errorf("class %d is %q, want %q", i, items[i].Label, want[i])
		}
	}
	if SpeedClass(3) != SpeedClassPhantom {
		t.Error("index 3 must be Phantom for row selection to map directly")
	}
}

// Track rows carry the internal id, which is what the per-track tables are keyed by.
func TestTrackEntriesMatchTheIDTable(t *testing.T) {
	table := []uint8{1, 8, 13, 20, 2, 17, 6, 7}
	if len(TrackMenuEntries) != len(table) {
		t.Fatalf("%d track entries, want %d", len(TrackMenuEntries), len(table))
	}
	for i, want := range table {
		if TrackMenuEntries[i].TrackID != want {
			t.Errorf("menu index %d has track id %d, want %d", i, TrackMenuEntries[i].TrackID, want)
		}
		if TrackMenuEntries[i].Description == "" || TrackMenuEntries[i].Rating == "" {
			t.Errorf("menu index %d is missing its retail text", i)
		}
	}
}

// Navigation pushes and pops, and Back from the main screen reports false so the
// caller knows to leave the front end.
func TestMenuNavigation(t *testing.T) {
	c := NewMenuCursor()
	if c.Screen() != ScreenMain {
		t.Fatal("does not open on the main screen")
	}
	if c.Back() {
		t.Error("Back from the main screen must report false")
	}
	ctx := &GameContext{}
	c.Move(1, ctx) // the TEAM column
	if c.Activate(ctx) != ActionNone || c.Screen() != ScreenTeam {
		t.Fatalf("activating TEAM went to %v", c.Screen())
	}
	c.Move(2, ctx) // AURICOM
	c.Activate(ctx)
	if ctx.TeamIndex != 2 {
		t.Errorf("team is %d, want 2", ctx.TeamIndex)
	}
	if c.Screen() != ScreenMain {
		t.Errorf("did not return to the main screen, on %v", c.Screen())
	}
	// The main screen remembered where the cursor was.
	if c.Selection() != 1 {
		t.Errorf("main selection is %d, want 1 where it was left", c.Selection())
	}
}

// Selecting a track sets both the menu index and the internal id, which are
// different values and have been a recurring source of bugs.
func TestTrackSelectionSetsBothFields(t *testing.T) {
	c := NewMenuCursor()
	ctx := &GameContext{AllTracksUnlocked: true}
	c.Move(2, ctx) // the CLASS AND TRACK column
	c.Activate(ctx)
	if c.Screen() != ScreenTrack {
		t.Fatalf("on %v, want the track screen", c.Screen())
	}
	c.Move(4, ctx) // Gare d'Europa, internal id 2
	c.Activate(ctx)
	if ctx.MenuTrackIndex != 4 || ctx.TrackID != 2 {
		t.Errorf("got menu %d / id %d, want 4 / 2", ctx.MenuTrackIndex, ctx.TrackID)
	}
}

// The track screen only offers what progression allows.
func TestTrackScreenRespectsUnlocks(t *testing.T) {
	c := NewMenuCursor()
	locked := &GameContext{}
	c.stack = append(c.stack, ScreenTrack)
	if n := c.itemCount(locked); n != 6 {
		t.Errorf("locked track count is %d, want 6", n)
	}
	if n := c.itemCount(&GameContext{AllTracksUnlocked: true}); n != 8 {
		t.Errorf("unlocked track count is %d, want 8", n)
	}
}

// START leaves the front end.
func TestStartItemReportsTheAction(t *testing.T) {
	c := NewMenuCursor()
	ctx := &GameContext{}
	c.Move(3, ctx) // START sits after the three columns
	if got := c.Activate(ctx); got != ActionStartRace {
		t.Errorf("START gave action %v, want ActionStartRace", got)
	}
}

// The teams come from the object names inside COMMON/TERRY.PRM, and the animal set
// from WIERD.PRM substitutes for them when that cheat is on.
func TestTeamSets(t *testing.T) {
	std := (&GameContext{}).Teams()
	if len(std) != 5 {
		t.Fatalf("%d standard teams, want 5", len(std))
	}
	wantObjects := []string{"quirex1", "fiesar1", "auricom2", "ag1", "piranha2"}
	for i, want := range wantObjects {
		if std[i].Object != want {
			t.Errorf("team %d model object is %q, want %q", i, std[i].Object, want)
		}
		if std[i].Name == "" {
			t.Errorf("team %d has no name", i)
		}
	}
	animals := (&GameContext{AnimalTeams: true}).Teams()
	if len(animals) != len(std) {
		t.Errorf("%d animal teams, want the same count as standard", len(animals))
	}
	if animals[0].Object != "snail" {
		t.Errorf("first animal model is %q, want snail", animals[0].Object)
	}
}

// Selecting a team records it, and the class models line up with SpeedClass.
func TestTeamSelectionAndClassModels(t *testing.T) {
	c := NewMenuCursor()
	ctx := &GameContext{}
	c.Move(1, ctx) // TEAM
	c.Activate(ctx)
	if c.Screen() != ScreenTeam {
		t.Fatalf("on %v, want the team screen", c.Screen())
	}
	c.Move(3, ctx) // AG SYSTEMS
	c.Activate(ctx)
	if ctx.TeamIndex != 3 {
		t.Errorf("team index is %d, want 3", ctx.TeamIndex)
	}
	for i, m := range ClassModels {
		if m.File == "" || m.Object == "" {
			t.Errorf("class %d has no model", i)
		}
	}
	if ClassModels[SpeedClassPhantom].Object != "phant" {
		t.Errorf("Phantom model is %q, want phant", ClassModels[SpeedClassPhantom].Object)
	}
}

// Every circuit must have a preview object, and the set must be exactly JUNE.PRM's
// eight with no repeats.
func TestTrackPreviewsCoverAllCircuits(t *testing.T) {
	seen := map[string]int{}
	for i, obj := range TrackPreviewObjects {
		if obj == "" {
			t.Errorf("menu index %d has no preview object", i)
			continue
		}
		seen[obj]++
	}
	if len(seen) != 8 {
		t.Errorf("%d distinct preview objects, want 8", len(seen))
	}
	for obj, n := range seen {
		if n > 1 {
			t.Errorf("%q is used for %d circuits", obj, n)
		}
	}
	// The internal names and directories must agree with the menu table.
	for i, entry := range TrackMenuEntries {
		if _, ok := TrackInternalNames[entry.TrackID]; !ok {
			t.Errorf("menu index %d (id %d) has no internal name", i, entry.TrackID)
		}
		if _, ok := TrackDirectories[entry.TrackID]; !ok {
			t.Errorf("menu index %d (id %d) has no directory", i, entry.TrackID)
		}
	}
}
