package game

import "testing"

// The boot sequence is fixed: four TIMs with the retail VSync hold counts, then the
// palanim overlay, then the title screen.
func TestBootSequenceReachesTheTitle(t *testing.T) {
	m := NewStateMachine()
	if m.State() != StateBootSplash {
		t.Fatalf("starts in %v, want boot splash", m.State())
	}
	seen := []string{}
	for i := 0; i < 10000; i++ {
		if s, ok := m.CurrentSplash(); ok {
			if len(seen) == 0 || seen[len(seen)-1] != s.Texture {
				seen = append(seen, s.Texture)
			}
		}
		m.Advance(false)
		if m.State() == StateTitleAttract {
			break
		}
	}
	if m.State() != StateTitleAttract {
		t.Fatalf("never reached the title, stuck in %v", m.State())
	}
	if len(seen) != len(BootSplashes) {
		t.Errorf("showed %d splashes (%v), want %d", len(seen), seen, len(BootSplashes))
	}
	if seen[1] != "TEXTURES/COPY2097.TIM" {
		t.Errorf("second splash is %q, want the copyright screen", seen[1])
	}
}

// The boot overlay is palanim, and it must be reported so a placeholder can name
// it rather than showing an unexplained black screen.
func TestBootOverlayIsPalanim(t *testing.T) {
	m := NewStateMachine()
	for i := 0; i < 10000 && m.State() != StateBootOverlay; i++ {
		m.Advance(false)
	}
	o, ok := m.CurrentOverlay()
	if !ok {
		t.Fatal("no overlay reported in the boot overlay state")
	}
	if o.File != `C:\palanim.exe` {
		t.Errorf("boot overlay is %q, want palanim", o.File)
	}
	if o.Name == "" {
		t.Error("overlay has no display name for the placeholder")
	}
}

// Retail times the title screen out into an unattended demo race after 5 seconds.
func TestTitleTimesOutIntoADemoRace(t *testing.T) {
	m := NewStateMachine()
	m.enter(StateTitleAttract)
	for i := 0; i < TitleTimeoutTicks-1; i++ {
		m.Advance(false)
	}
	if m.State() != StateTitleAttract {
		t.Fatalf("left the title after %d ticks, too early", m.Tick())
	}
	m.Advance(false)
	if m.State() != StateRace {
		t.Fatalf("state is %v after the timeout, want race", m.State())
	}
	if !m.IsDemo() {
		t.Error("a timed-out race must be flagged as a demo")
	}
}

// Start is ignored until the debounce has elapsed, so a button held over from a
// previous screen cannot skip the title instantly.
func TestStartIsDebounced(t *testing.T) {
	m := NewStateMachine()
	m.enter(StateTitleAttract)
	m.Advance(true)
	if m.State() != StateTitleAttract {
		t.Fatal("start accepted on the first tick, before the debounce")
	}
	for i := 0; i < TitleInputDebounceTicks; i++ {
		m.Advance(true)
	}
	if m.State() != StateFrontEnd {
		t.Fatalf("state is %v, want front end after start", m.State())
	}
	if m.IsDemo() {
		t.Error("pressing start must not produce a demo race")
	}
}

// The front end returning 1 means the player backed out.
func TestFrontEndReturnValueRouting(t *testing.T) {
	m := NewStateMachine()
	m.enter(StateFrontEnd)
	m.FrontEndResult(RaceSetupBackToTitle)
	if m.State() != StateTitleAttract {
		t.Errorf("setup 1 went to %v, want the title", m.State())
	}
	m.enter(StateFrontEnd)
	m.FrontEndResult(4)
	if m.State() != StateRace {
		t.Errorf("setup 4 went to %v, want a race", m.State())
	}
}

// Retail checks the cutscenes in the order 1, 3, 2, 4 -- not numerically -- and
// plays at most one per race.
func TestPostRaceOverlayOrderIsOneThreeTwoFour(t *testing.T) {
	m := NewStateMachine()
	m.enter(StateRace)
	// Request xtro2 and xtro3; xtro3 must win because it is checked first.
	m.RequestPostRaceOverlay(1) // xtro3
	m.RequestPostRaceOverlay(2) // xtro2
	m.RaceFinished()
	o, ok := m.CurrentOverlay()
	if !ok {
		t.Fatal("no overlay after the race")
	}
	if o.File != `c:\xtro3.exe` {
		t.Errorf("played %q, want xtro3 to be checked before xtro2", o.File)
	}
	// The still-pending xtro2 plays after the next race.
	for m.State() == StatePostRaceOverlay {
		m.Advance(false)
	}
	m.enter(StateRace)
	m.RaceFinished()
	o, _ = m.CurrentOverlay()
	if o.File != `c:\xtro2.exe` {
		t.Errorf("second race played %q, want the still-pending xtro2", o.File)
	}
}

// xtro1 and xtro3 latch after playing once; xtro2 and xtro4 do not.
func TestPlayOnceLatchOnlyAppliesToTheFirstTwo(t *testing.T) {
	m := NewStateMachine()
	m.enter(StateRace)
	m.RequestPostRaceOverlay(0)
	m.RaceFinished()
	if o, _ := m.CurrentOverlay(); o.File != `c:\xtro1.exe` {
		t.Fatalf("first play was %q", o.File)
	}
	for m.State() == StatePostRaceOverlay {
		m.Advance(false)
	}
	// Requesting it again must not replay it.
	m.enter(StateRace)
	m.RequestPostRaceOverlay(0)
	m.RaceFinished()
	if m.State() == StatePostRaceOverlay {
		t.Error("xtro1 replayed despite its shown latch")
	}

	// xtro4 has no latch, so it can recur.
	n := NewStateMachine()
	for i := 0; i < 2; i++ {
		n.enter(StateRace)
		n.RequestPostRaceOverlay(3)
		n.RaceFinished()
		if n.State() != StatePostRaceOverlay {
			t.Fatalf("xtro4 did not play on pass %d", i)
		}
		for n.State() == StatePostRaceOverlay {
			n.Advance(false)
		}
	}
}

// With nothing pending, a finished race goes straight back to the title.
func TestRaceWithoutOverlayReturnsToTitle(t *testing.T) {
	m := NewStateMachine()
	m.enter(StateRace)
	m.RaceFinished()
	if m.State() != StateTitleAttract {
		t.Errorf("state is %v, want the title", m.State())
	}
}

// The PRESS START blink is on for 18 of every 25 ticks.
func TestPressStartBlinkDutyCycle(t *testing.T) {
	on := 0
	for tick := 0; tick < pressStartBlinkPeriod; tick++ {
		if PressStartVisible(tick) {
			on++
		}
	}
	if on != pressStartBlinkOn {
		t.Errorf("visible for %d of %d ticks, want %d", on, pressStartBlinkPeriod, pressStartBlinkOn)
	}
	if !PressStartVisible(0) {
		t.Error("blink should start lit")
	}
	if PressStartVisible(pressStartBlinkOn) {
		t.Error("blink should be dark at the end of the period")
	}
}

// The now-playing banner is indexed as selection-1, and the tables are parallel.
func TestNowPlayingBanner(t *testing.T) {
	if len(MusicTracks) != 12 {
		t.Fatalf("%d music entries, want 12 (RANDOM plus 11 tracks)", len(MusicTracks))
	}
	if MusicTracks[0].Title != "RANDOM" {
		t.Errorf("entry 0 is %q; retail keeps the RANDOM label there", MusicTracks[0].Title)
	}
	got := NowPlaying(7)
	want := "CD TRACK: 6   LOOPS OF FURY [CHEMICAL BROTHERS]"
	if got != want {
		t.Errorf("NowPlaying(7) = %q, want %q", got, want)
	}
	if NowPlaying(0) != "" {
		t.Error("selection 0 has no track and must render nothing")
	}
	if NowPlaying(99) != "" {
		t.Error("out-of-range selection must render nothing")
	}
}

// Retail clamps the menu track index and refuses Phantom until it is unlocked,
// keeping the internal track id in step with the menu index.
func TestGameContextNormalise(t *testing.T) {
	table := []uint8{1, 8, 13, 20, 2, 17, 6, 7}

	c := GameContext{MenuTrackIndex: 0}
	c.Normalise(table)
	if c.TrackID != 1 {
		t.Errorf("menu index 0 gave track id %d, want 1 (Talon's Reach)", c.TrackID)
	}

	c = GameContext{MenuTrackIndex: 9}
	c.Normalise(table)
	if c.MenuTrackIndex != 0 || c.TrackID != 1 {
		t.Errorf("out-of-range index gave %d/%d, want 0/1", c.MenuTrackIndex, c.TrackID)
	}

	// The two VERY HARD tracks open with Challenge II or the track cheat.
	c = GameContext{MenuTrackIndex: 7}
	c.Normalise(table)
	if c.MenuTrackIndex != 0 {
		t.Errorf("track 7 stayed selectable at %d without an unlock", c.MenuTrackIndex)
	}
	c = GameContext{MenuTrackIndex: 7, AllTracksUnlocked: true}
	c.Normalise(table)
	if c.MenuTrackIndex != 7 || c.TrackID != 7 {
		t.Errorf("track cheat gave %d/%d, want 7/7", c.MenuTrackIndex, c.TrackID)
	}
	c = GameContext{MenuTrackIndex: 6, Challenge2Flag: true}
	c.Normalise(table)
	if c.MenuTrackIndex != 6 || c.TrackID != 6 {
		t.Errorf("Challenge II gave %d/%d, want 6/6", c.MenuTrackIndex, c.TrackID)
	}
	if (&GameContext{}).SelectableTrackCount() != 6 {
		t.Error("base selectable count should be 6")
	}
	if (&GameContext{AllTracksUnlocked: true}).SelectableTrackCount() != 8 {
		t.Error("track cheat should offer 8")
	}

	c = GameContext{MenuTrackIndex: 4}
	c.Normalise(table)
	if c.TrackID != 2 {
		t.Errorf("menu index 4 gave track id %d, want 2", c.TrackID)
	}
}

// Volumes are stored as percentages and converted to 0..255 at every retail call
// site.
func TestAudioLevelConversion(t *testing.T) {
	for _, tc := range []struct {
		percent uint16
		want    uint8
	}{{0, 0}, {50, 127}, {100, 255}, {200, 255}} {
		if got := AudioLevel(tc.percent); got != tc.want {
			t.Errorf("AudioLevel(%d) = %d, want %d", tc.percent, got, tc.want)
		}
	}
}

func TestStateNamesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for s := StateBootSplash; s <= StatePostRaceOverlay; s++ {
		n := s.String()
		if n == "" || seen[n] {
			t.Errorf("state %d has a missing or duplicate name %q", int(s), n)
		}
		seen[n] = true
	}
}

// postRaceOverlayCount has to be a constant, so it cannot be derived from the
// table. Pin them together.
func TestOverlayFlagArraysMatchTheTable(t *testing.T) {
	if len(PostRaceOverlays) != postRaceOverlayCount {
		t.Fatalf("postRaceOverlayCount is %d but the table has %d entries",
			postRaceOverlayCount, len(PostRaceOverlays))
	}
}
