package game

import "fmt"

// This file ports WipeOut 2097's top-level state machine from SLES_003.27 (PAL).
// The shape comes from main (0x8001a464); see
// bn-psx/docs/wipeout2097_top_level_state_machine.md for the disassembly it was
// read from. Retail runs a 25 Hz tick (its frame-end limiter waits two PAL
// fields), so every duration here is in ticks at 25 Hz.

// TicksPerSecond matches retail's PAL tick rate.
const TicksPerSecond = 25

// TopLevelState enumerates the states main cycles through. Retail has no such
// enum -- it uses a local flag plus maybe_SelectedRaceSetup (0x80095830) as the
// discriminator, and writes 2 to maybe_TopLevelStateByte (0x80095764) at each
// transition. Naming the states makes the flow testable.
type TopLevelState int

const (
	// StateBootSplash shows the fixed sequence of boot TIMs.
	StateBootSplash TopLevelState = iota
	// StateBootOverlay runs C:\palanim.exe, loaded from a stack buffer in main.
	StateBootOverlay
	// StateTitleAttract blinks PRESS START and times out into a demo race.
	StateTitleAttract
	// StateFrontEnd is maybe_FrontEndMainLoop (0x8004e490); its return value
	// becomes the race setup, and returning 1 goes back to the title.
	StateFrontEnd
	// StateRace is maybe_RaceMain (0x8003f494).
	StateRace
	// StatePostRaceOverlay runs at most one xtro cutscene, then returns to the
	// title.
	StatePostRaceOverlay
)

func (s TopLevelState) String() string {
	switch s {
	case StateBootSplash:
		return "boot splash"
	case StateBootOverlay:
		return "boot overlay"
	case StateTitleAttract:
		return "title/attract"
	case StateFrontEnd:
		return "front end"
	case StateRace:
		return "race"
	case StatePostRaceOverlay:
		return "post-race overlay"
	}
	return fmt.Sprintf("state(%d)", int(s))
}

// BootSplash is one entry of the boot TIM sequence. Retail loads each image and
// then spins on VSync, so the wait *follows* the load it belongs to.
type BootSplash struct {
	Texture string
	// HoldVSyncs is the retail loop bound, in 50 Hz PAL fields.
	HoldVSyncs int
}

// HoldTicks converts the retail field count to game ticks. VSync counts fields at
// 50 Hz while the game state advances at 25 Hz, so a 240-field wait is 120 ticks
// (4.8 seconds), not 240.
func (b BootSplash) HoldTicks() int { return b.HoldVSyncs / 2 }

// BootSplashes is the sequence main runs before any loop, with the literal VSync
// loop bounds that follow each load: 0xf0, 0x32, 0x32, 0x64.
//
// Confirmed against a retail boot in an emulator, which shows the piracy warning,
// the WipeOut 2097 copyright screen, then the Red Bull screen -- and no Dolby screen
// at all. DOLBYPAL.TIM is not on the disc, so retail's load fails and the copyright
// screen simply stays up through that slot's 50 fields, because nothing clears the
// framebuffer. A caller must reproduce that by holding the previous image rather
// than blanking.
var BootSplashes = []BootSplash{
	// The piracy warning. This file is 640x256 where the others are 320x256.
	{Texture: "TEXTURES/WARNING.TIM", HoldVSyncs: 240},
	// The WipeOut 2097 copyright screen.
	{Texture: "TEXTURES/COPY2097.TIM", HoldVSyncs: 50},
	// Requested by the executable but absent from the disc; the previous screen
	// persists here.
	{Texture: "TEXTURES/DOLBYPAL.TIM", HoldVSyncs: 50},
	// The Red Bull screen. "REDB" is Red Bull; REDBNTSC.TIM is the NTSC twin.
	{Texture: "TEXTURES/REDBPAL.TIM", HoldVSyncs: 100},
}

// TitleTexture is loaded when the title screen is (re)entered. Retail also has this
// on screen while the boot overlay runs, which is the "loading screen" a player sees
// between the Red Bull screen and the title, so a placeholder for an overlay should
// draw it rather than blanking to black.
const TitleTexture = "TEXTURES/STARTPAL.TIM"

// Overlay is a separate PS-EXE that retail loads over itself via PsyQ Exec.
// maybe_LoadAndRunOverlay (0x80067ac0) takes the filename, loads the header to
// 0x80119000 and the body to 0x80118800, tears down pad and watchdog, execs, then
// reinitialises everything on return.
//
// A port cannot execute these, and their content is FMV (the disc carries
// XTRO1.AV and MAKE.AV). Until video playback exists, ShowAsPlaceholder renders
// the name on a black screen.
type Overlay struct {
	// File is the retail path, kept verbatim for traceability.
	File string
	// Name is what a placeholder screen displays.
	Name string
	// PendingAddr and ShownAddr are the retail $gp-relative flags, recorded so the
	// mapping back to the executable stays checkable. ShownAddr is 0 where retail
	// has no play-once latch.
	PendingAddr uint32
	ShownAddr   uint32
}

// BaseName is the overlay's filename without its drive or directory, uppercased.
// The retail font has no glyph for a backslash, so the full path cannot be drawn;
// this is what a placeholder screen should show.
func (o Overlay) BaseName() string {
	name := o.File
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '\\' || name[i] == '/' || name[i] == ':' {
			name = name[i+1:]
			break
		}
	}
	out := []byte(name)
	for i := range out {
		if out[i] >= 'a' && out[i] <= 'z' {
			out[i] -= 'a' - 'A'
		}
	}
	return string(out)
}

// BootOverlay is launched once at the end of main's boot sequence.
var BootOverlay = Overlay{File: `C:\palanim.exe`, Name: "PALETTE ANIMATION"}

// PostRaceOverlays is the if/else-if chain main runs immediately after
// maybe_RaceMain returns. Only the first eligible one plays.
//
// The order is deliberate and is NOT numerical: retail checks xtro1, then xtro3,
// then xtro2, then xtro4. The first two have a "shown" latch so they play once
// ever; the last two only have a pending flag, so they can recur.
var PostRaceOverlays = []Overlay{
	{File: `c:\xtro1.exe`, Name: "INTRO SEQUENCE 1", PendingAddr: 0x80094918, ShownAddr: 0x8009491a},
	{File: `c:\xtro3.exe`, Name: "INTRO SEQUENCE 3", PendingAddr: 0x8009491c, ShownAddr: 0x8009491e},
	{File: `c:\xtro2.exe`, Name: "INTRO SEQUENCE 2", PendingAddr: 0x80094920},
	{File: `c:\xtro4.exe`, Name: "INTRO SEQUENCE 4", PendingAddr: 0x80094922},
}

// postRaceOverlayCount sizes the flag arrays. A test asserts it matches the table,
// since Go will not let a slice length be used as an array bound.
const postRaceOverlayCount = 4

// PlaceholderOverlayTicks is how long a placeholder screen shows in place of an
// overlay. This is a port decision, not a retail duration -- retail runs until the
// overlay's own executable exits.
const PlaceholderOverlayTicks = 5 * TicksPerSecond

// MusicTrack pairs a title with its performer. maybe_MusicTrackTitles
// (0x80091660) and maybe_MusicTrackArtists (0x80091694) are parallel char*
// arrays; entry 0 is the literal "RANDOM" menu label rather than a track, and
// callers index them as selection-1.
type MusicTrack struct {
	Title  string
	Artist string
}

// MusicTracks is indexed the way retail indexes it: [0] is the RANDOM label.
var MusicTracks = []MusicTrack{
	{Title: "RANDOM", Artist: "RANDOM"},
	{Title: "WE HAVE EXPLOSIVE", Artist: "F.S.O.L."},
	{Title: "LANDMASS", Artist: "F.S.O.L."},
	{Title: "ATOMBOMB INSTR.", Artist: "FLUKE"},
	{Title: "V6", Artist: "FLUKE"},
	{Title: "DUST UP BEATS", Artist: "CHEMICAL BROTHERS"},
	{Title: "LOOPS OF FURY", Artist: "CHEMICAL BROTHERS"},
	{Title: "THE THIRD SEQUENCE", Artist: "PHOTEK"},
	{Title: "TIN THERE [EDIT]", Artist: "UNDERWORLD"},
	{Title: "FIRESTARTER INSTR.", Artist: "THE PRODIGY"},
	{Title: "CANADA", Artist: "COLD STORAGE"},
	{Title: "BODY IN MOTION", Artist: "COLD STORAGE"},
}

// MusicSelectionRandom is the value of GameContext.MusicSelection that means
// "pick from the shuffled order" rather than naming a track.
const MusicSelectionRandom = 1

// NowPlayingFormat is the retail format string at 0x80016ebc, used for the banner
// shown briefly at race start.
const NowPlayingFormat = "CD TRACK: %d   %s [%s]"

// NowPlaying renders the banner for a resolved track selection, which is the value
// retail stores in g_80095728 and then indexes as selection-1.
func NowPlaying(selection int) string {
	index := selection - 1
	if index < 0 || index >= len(MusicTracks) {
		return ""
	}
	return fmt.Sprintf(NowPlayingFormat, index, MusicTracks[index].Title, MusicTracks[index].Artist)
}

// SpeedClass is GameContext.SpeedClass (+0x08). The select screen labels the four
// values NOVICE, INTERMEDIATE, EXPERT and SPEED DEMON. Whether the fourth is gated
// is NOT established: the clamp previously believed to do that turned out to clamp
// the track index instead.
type SpeedClass uint8

const (
	SpeedClassVector SpeedClass = 0
	SpeedClassVenom  SpeedClass = 1
	SpeedClassRapier SpeedClass = 2
	// SpeedClassPhantom is "SPEED DEMON" on the select screen.
	SpeedClassPhantom SpeedClass = 3
)

// GameContext mirrors the struct maybe_GameContext (0x80095720) points at: every
// current selection, audio setting and unlock flag. In retail it is a stack buffer
// in main that lives for the whole program.
//
// Only fields with observed accesses in the executable are modelled. Field offsets
// are in the doc; they are not reproduced as struct tags because nothing here
// serialises to the retail layout.
type GameContext struct {
	// MenuTrackIndex is the position in menu order (+0x04).
	MenuTrackIndex uint8
	// TrackID is the internal id (+0x05), derived as
	// MenuIndexToTrackID[MenuTrackIndex]. The per-track tables are keyed by this,
	// never by the menu index.
	TrackID uint8
	// SpeedClass is +0x08.
	SpeedClass SpeedClass
	// MusicSelection is +0x10; MusicSelectionRandom means use the shuffled order.
	MusicSelection int8
	// MusicVolumePercent and SFXVolumePercent are +0x1e and +0x20. Retail converts
	// them with value * 0xff / 100 before handing them to the audio driver.
	MusicVolumePercent uint16
	SFXVolumePercent   uint16
	// Challenge1Flag and Challenge2Flag are +0x39 and +0x3a; they select the
	// "CHALLENGE I" / "CHALLENGE II" label.
	Challenge1Flag bool
	Challenge2Flag bool
	// AllTracksUnlocked is +0x62, set by the Eight Tracks cheat (hold L1+R1+Select,
	// press Square, Circle, Triangle, Circle, Square) and equivalent to earning
	// Challenge II. maybe_TrackSelectScreen draws "TRACK CHEAT ACTIVE" from it and
	// raises the selectable track count to 8.
	AllTracksUnlocked bool
	// RaceTypeIndex is the selected race type: the row index into
	// Screens[ScreenRaceType]. The retail race mode lives separately in
	// maybe_RaceModeSelection (0x80095770) rather than in this struct.
	RaceTypeIndex uint8
	// TeamIndex is the selected team. It has not been located in the retail context
	// struct, so this is the port's own field rather than a mirrored offset.
	TeamIndex uint8
	// AnimalTeams substitutes the WIERD.PRM craft for the standard teams. Retail
	// calls it "silly ships" and gates it on maybe_SillyShipsCheatEnabled
	// (0x8009495c); the published route in is holding L1+R2+Start+Select while the
	// game loads.
	AnimalTeams bool
	// PhantomTrackCheat is +0x63, set by Triangle x3, Circle x3. The select screen
	// labels it "PHANTOM TRACK CHEAT ACTIVE". What it actually grants is not yet
	// established -- it sets the selectable count to 2 or 6, not 8.
	PhantomTrackCheat bool
}

// menuTrackCount is the base number of selectable tracks. MenuIndexToTrackID has 8
// entries; the last two -- both rated VERY HARD by the select screen -- open only
// with Challenge II or the track cheat.
const menuTrackCount = 6

// Normalise applies the clamp maybe_InitChallengeModeSettings performs and keeps
// TrackID consistent with MenuTrackIndex:
//
//	if (menuTrackIndex < 6)  ok;      // sltiu $v0, $v0, 6
//	if (field_3e != 0)       ok;
//	if (allTracksUnlocked)   ok;      // +0x62
//	menuTrackIndex = 0; trackId = 1;
//
// Note this clamps the TRACK INDEX, not the speed class. An earlier reading of this
// function had it clamping speedClass on a flag named phantomClassUnlocked; the
// disassembly does not support that, and the external cheat list corroborates +0x62
// as the all-eight-tracks unlock.
func (c *GameContext) Normalise(menuIndexToTrackID []uint8) {
	if int(c.MenuTrackIndex) >= menuTrackCount && !c.AllTracksUnlocked && !c.Challenge2Flag {
		c.MenuTrackIndex = 0
		c.TrackID = 1
	}
	if int(c.MenuTrackIndex) < len(menuIndexToTrackID) {
		c.TrackID = menuIndexToTrackID[c.MenuTrackIndex]
	}
}

// SelectableTrackCount is how many tracks the select screen offers.
// maybe_TrackSelectScreen raises it to 8 when Challenge II is earned or the track
// cheat is set, and otherwise offers the base six.
func (c *GameContext) SelectableTrackCount() int {
	if c.Challenge2Flag || c.AllTracksUnlocked {
		return 8
	}
	return menuTrackCount
}

// AudioLevel converts a stored percentage to the 0..255 the driver takes, the way
// retail does at every call site: `value * 0xff / 100`.
func AudioLevel(percent uint16) uint8 {
	if percent > 100 {
		percent = 100
	}
	return uint8(uint32(percent) * 0xff / 100)
}

// Title screen timing, from main's attract loop.
const (
	// TitleInputDebounceTicks is the `frames >= 0xb` gate before START is read.
	TitleInputDebounceTicks = 11
	// TitleTimeoutTicks is the normal 5-second timeout into a demo race. Retail
	// computes it as threshold * 25 with threshold 5.
	TitleTimeoutTicks = 5 * TicksPerSecond
	// TitleTimeoutTicksLong is the 30-second variant retail uses on one path
	// (threshold 0x1e).
	TitleTimeoutTicksLong = 30 * TicksPerSecond
	// pressStartBlinkPeriod and pressStartBlinkOn implement the retail blink:
	// the text draws while (frame % 25) < 0x12.
	pressStartBlinkPeriod = 25
	pressStartBlinkOn     = 0x12
)

// PressStartVisible reproduces the retail blink duty cycle: on for 18 of every 25
// ticks, so roughly 0.72 s lit and 0.28 s dark.
func PressStartVisible(tick int) bool {
	return tick%pressStartBlinkPeriod < pressStartBlinkOn
}

// Values maybe_FrontEndMainLoop returns, which main treats as a discriminator rather
// than as data.
const (
	// RaceSetupBackToTitle means the player backed out; main continues its loop and the
	// title screen comes back.
	RaceSetupBackToTitle = 1
	// RaceSetupStartRace means start a race with the current context.
	//
	// It must not be 1. Passing a track id here instead was a bug: Talon's Reach is
	// internal id 1, so selecting it collided with RaceSetupBackToTitle and pressing
	// START returned to the attract screen rather than racing. The track, class and team
	// all live in GameContext, so nothing needs to travel in this value.
	RaceSetupStartRace = 2
)

// StateMachine drives the top-level flow. It owns no rendering: a caller asks for
// the current state each tick and draws accordingly, then reports events back.
type StateMachine struct {
	// Context is the persistent selections and settings.
	Context GameContext

	state TopLevelState
	// tick counts ticks spent in the current state, reset on entry.
	tick int
	// splash indexes BootSplashes while in StateBootSplash.
	splash int
	// overlay is the overlay awaiting or showing a placeholder.
	overlay Overlay
	// pending marks which post-race overlays are requested, and shown latches the
	// two that play only once.
	pending [postRaceOverlayCount]bool
	shown   [postRaceOverlayCount]bool
	// demo is set when the title timed out, so the race runs unattended.
	demo bool
	// titleTimeout allows the 30-second variant to be selected.
	titleTimeout int
}

// NewStateMachine starts at the first boot splash, as retail does.
func NewStateMachine() *StateMachine {
	return &StateMachine{
		state:        StateBootSplash,
		titleTimeout: TitleTimeoutTicks,
		Context: GameContext{
			MusicSelection:     MusicSelectionRandom,
			MusicVolumePercent: 100,
			SFXVolumePercent:   100,
		},
	}
}

// State reports the current state.
func (m *StateMachine) State() TopLevelState { return m.state }

// Tick returns how many ticks have elapsed in the current state.
func (m *StateMachine) Tick() int { return m.tick }

// IsDemo reports whether the upcoming or current race is an unattended demo,
// which retail enters by letting the title screen time out.
func (m *StateMachine) IsDemo() bool { return m.demo }

// SplashIndex is which entry of BootSplashes is showing, so a caller can index a
// parallel array of uploaded textures. It is -1 outside StateBootSplash.
func (m *StateMachine) SplashIndex() int {
	if m.state != StateBootSplash || m.splash >= len(BootSplashes) {
		return -1
	}
	return m.splash
}

// CurrentSplash returns the splash being shown, valid in StateBootSplash.
func (m *StateMachine) CurrentSplash() (BootSplash, bool) {
	if m.state != StateBootSplash || m.splash >= len(BootSplashes) {
		return BootSplash{}, false
	}
	return BootSplashes[m.splash], true
}

// CurrentOverlay returns the overlay whose placeholder is showing, valid in the
// two overlay states.
func (m *StateMachine) CurrentOverlay() (Overlay, bool) {
	if m.state != StateBootOverlay && m.state != StatePostRaceOverlay {
		return Overlay{}, false
	}
	return m.overlay, true
}

// RequestPostRaceOverlay marks one of the xtro cutscenes as pending, mirroring a
// retail write to its $gp flag. Out-of-range indices are ignored.
func (m *StateMachine) RequestPostRaceOverlay(index int) {
	if index >= 0 && index < len(m.pending) {
		m.pending[index] = true
	}
}

// SetTitleTimeoutLong selects retail's 30-second title timeout instead of 5.
func (m *StateMachine) SetTitleTimeoutLong(long bool) {
	if long {
		m.titleTimeout = TitleTimeoutTicksLong
		return
	}
	m.titleTimeout = TitleTimeoutTicks
}

func (m *StateMachine) enter(s TopLevelState) {
	m.state = s
	m.tick = 0
}

// Advance runs one tick. pressStart reports whether the player pressed start this
// tick; it is only consulted on the title screen.
func (m *StateMachine) Advance(pressStart bool) {
	m.tick++
	switch m.state {
	case StateBootSplash:
		if m.splash < len(BootSplashes) && m.tick >= BootSplashes[m.splash].HoldTicks() {
			m.splash++
			m.tick = 0
			if m.splash >= len(BootSplashes) {
				m.overlay = BootOverlay
				m.enter(StateBootOverlay)
			}
		}
	case StateBootOverlay:
		if m.tick >= PlaceholderOverlayTicks {
			m.enter(StateTitleAttract)
		}
	case StateTitleAttract:
		// Retail debounces before reading start, so a button still held from a
		// previous screen cannot fall straight through.
		if pressStart && m.tick >= TitleInputDebounceTicks {
			m.demo = false
			m.enter(StateFrontEnd)
			return
		}
		if m.tick >= m.titleTimeout {
			m.demo = true
			m.enter(StateRace)
		}
	case StateFrontEnd, StateRace, StatePostRaceOverlay:
		// These end on an explicit event rather than a timer, except the overlay
		// placeholder which is timed.
		if m.state == StatePostRaceOverlay && m.tick >= PlaceholderOverlayTicks {
			m.enter(StateTitleAttract)
		}
	}
}

// FrontEndResult reports what the front end returned. RaceSetupBackToTitle sends
// the machine back to the title; anything else starts a race.
func (m *StateMachine) FrontEndResult(setup int) {
	if m.state != StateFrontEnd {
		return
	}
	if setup == RaceSetupBackToTitle {
		m.enter(StateTitleAttract)
		return
	}
	m.demo = false
	m.enter(StateRace)
}

// RaceFinished ends the race. It runs the first eligible post-race overlay,
// following retail's 1, 3, 2, 4 check order, and otherwise returns to the title.
func (m *StateMachine) RaceFinished() {
	if m.state != StateRace {
		return
	}
	for i := range PostRaceOverlays {
		if !m.pending[i] {
			continue
		}
		// The first two entries carry a play-once latch; the others do not.
		if PostRaceOverlays[i].ShownAddr != 0 && m.shown[i] {
			continue
		}
		m.pending[i] = false
		if PostRaceOverlays[i].ShownAddr != 0 {
			m.shown[i] = true
		}
		m.overlay = PostRaceOverlays[i]
		m.enter(StatePostRaceOverlay)
		return
	}
	m.enter(StateTitleAttract)
}
