package game

// The front end's screens, transcribed from maybe_BuildRaceSetupMenuLabels
// (SLES_003.27 0x8005e6e0), which fills maybe_MenuData (0x80095864) with every
// label the menus draw. Screens sit 0x46e apart in that structure and items 0x88
// apart within a screen, which is how the offsets below were grouped.

// MenuItem is one selectable row.
type MenuItem struct {
	// Label is the retail string, verbatim.
	Label string
	// Target is the screen this opens, or ScreenNone for an item that acts rather
	// than navigating.
	Target MenuScreenID
	// Action is what an acting item does.
	Action MenuAction
}

// MenuScreenID identifies a screen.
type MenuScreenID int

const (
	ScreenNone MenuScreenID = iota
	ScreenMain
	ScreenRaceType
	ScreenClass
	ScreenTeam
	ScreenTrack
	ScreenOptions
)

// MenuAction is what a non-navigating item does.
type MenuAction int

const (
	ActionNone MenuAction = iota
	// ActionStartRace leaves the front end with the current selections.
	ActionStartRace
	// ActionBack returns to the parent screen, or out of the front end from the
	// main screen -- which is what makes maybe_FrontEndMainLoop return 1.
	ActionBack
)

// MenuScreen is one screen: a title and its items.
type MenuScreen struct {
	// Title is the heading retail stores separately from the items.
	Title string
	Items []MenuItem
}

// Screens is the front end's structure. Only the branches whose labels appear in
// maybe_BuildRaceSetupMenuLabels are modelled; the link-up and controller-config
// subtrees exist in retail but are not transcribed.
//
// The ordering within each screen follows the ascending 0x88-spaced offsets the
// labels are written to, which is the order they are drawn in.
var Screens = map[MenuScreenID]MenuScreen{
	ScreenMain: {
		// Retail writes "RACE" and "TYPE" as two separate strings, drawn on two
		// lines for one item.
		Title: "WIPEOUT",
		Items: []MenuItem{
			{Label: "RACE TYPE", Target: ScreenRaceType},
			{Label: "TEAM", Target: ScreenTeam},
			{Label: "CLASS", Target: ScreenClass},
			{Label: "TRACK", Target: ScreenTrack},
			{Label: "OPTIONS", Target: ScreenOptions},
			{Label: "START", Action: ActionStartRace},
		},
	},
	ScreenRaceType: {
		Title: "RACE TYPE",
		Items: []MenuItem{
			{Label: "ARCADE"},
			{Label: "TIME TRIAL"},
			{Label: "ARCADE LINK"},
			{Label: "ONE ON ONE"},
			// The label is rewritten to "CHALLENGE II" once that is unlocked; see
			// maybe_InitChallengeModeSettings, which also sets menuData+0x8d0 = 5.
			{Label: "CHALLENGE I"},
		},
	},
	ScreenClass: {
		Title: "RACING CLASS",
		Items: []MenuItem{
			{Label: "VECTOR"},
			{Label: "VENOM"},
			{Label: "RAPIER"},
			{Label: "PHANTOM"},
		},
	},
	ScreenTeam: {
		Title: "TEAM MENU",
		// Team names are not in maybe_BuildRaceSetupMenuLabels -- that function only
		// writes the "TEAM" item and this screen's heading. They come from elsewhere
		// and have not been located, so this screen has no items yet.
	},
	ScreenTrack: {
		Title: "TRACK",
		// Populated at runtime from the track table, since availability depends on
		// progression. See TrackMenuEntries.
	},
	ScreenOptions: {
		Title: "OPTIONS MENU",
		Items: []MenuItem{
			{Label: "AUDIO CONFIG"},
			{Label: "CONTROLLER CONFIG"},
			{Label: "PREFERENCES"},
			{Label: "LOAD AND SAVE"},
			{Label: "PASSWORD"},
			{Label: "BEST ARCADE TIMES"},
			{Label: "BEST TIME TRIAL TIMES"},
		},
	},
}

// TrackMenuEntry is one row of the track select screen. The descriptions and
// ratings are the strings maybe_TrackSelectScreen (0x8005b9f0) draws.
type TrackMenuEntry struct {
	// Name is not in the executable's track select strings, which describe rather
	// than name each circuit. These are the published names, in menu order.
	Name string
	// Description and Rating are retail's own text.
	Description string
	Rating      string
	// TrackID is the internal id, from MenuIndexToTrackID.
	TrackID uint8
}

// TrackMenuEntries is the eight circuits in menu order. The last two are the ones
// the `menuTrackIndex >= 6` clamp hides until Challenge II or the track cheat.
var TrackMenuEntries = []TrackMenuEntry{
	{Name: "TALONS REACH", Description: "A MAJOR CANADIAN INDUSTRIAL COMPLEX.", Rating: "EASY", TrackID: 1},
	{Name: "SAGARMATHA", Description: "A SNOWY TIBETAN MOUNTAIN CIRCUIT.", Rating: "EASY", TrackID: 8},
	{Name: "VALPARAISO", Description: "A SOUTH AMERICAN JUNGLE CIRCUIT.", Rating: "TRICKY", TrackID: 13},
	{Name: "PHENITIA PARK", Description: "A NEWLY CONSTRUCTED GERMAN COMMERCIAL PARK.", Rating: "TRICKY", TrackID: 20},
	{Name: "GARE DEUROPA", Description: "A DISUSED FRENCH METRO SYSTEM.", Rating: "HARD", TrackID: 2},
	{Name: "ODESSA KEYS", Description: "A HUGE SUSPENDED CIRCUIT OVER THE BLACK SEA.", Rating: "HARD", TrackID: 17},
	{Name: "VOSTOK ISLAND", Description: "AN OBSCURE VOLCANIC ISLAND LOCATED IN THE SOUTH PACIFIC.", Rating: "VERY HARD", TrackID: 6},
	{Name: "SPILSKINANKE", Description: "AN AMERICAN CITY CIRCUIT AFFECTED BY SEISMIC ACTIVITY.", Rating: "VERY HARD", TrackID: 7},
}

// MenuCursor tracks the player's position in the screen stack.
type MenuCursor struct {
	// stack is the path from the main screen down, so Back can unwind it.
	stack []MenuScreenID
	// selection is the highlighted row per screen, remembered across visits the way
	// retail mirrors its cursor into maybe_MenuData.
	selection map[MenuScreenID]int
}

// NewMenuCursor opens the main screen.
func NewMenuCursor() *MenuCursor {
	return &MenuCursor{
		stack:     []MenuScreenID{ScreenMain},
		selection: map[MenuScreenID]int{},
	}
}

// Screen is the screen currently displayed.
func (c *MenuCursor) Screen() MenuScreenID {
	if len(c.stack) == 0 {
		return ScreenMain
	}
	return c.stack[len(c.stack)-1]
}

// Selection is the highlighted row on the current screen.
func (c *MenuCursor) Selection() int { return c.selection[c.Screen()] }

// itemCount is how many rows the current screen has, counting the runtime-built
// track list.
func (c *MenuCursor) itemCount(ctx *GameContext) int {
	if c.Screen() == ScreenTrack {
		if ctx != nil {
			return ctx.SelectableTrackCount()
		}
		return len(TrackMenuEntries)
	}
	return len(Screens[c.Screen()].Items)
}

// Move steps the highlight, wrapping at both ends.
func (c *MenuCursor) Move(delta int, ctx *GameContext) {
	n := c.itemCount(ctx)
	if n == 0 {
		return
	}
	s := (c.selection[c.Screen()] + delta) % n
	if s < 0 {
		s += n
	}
	c.selection[c.Screen()] = s
}

// Activate presses the highlighted row. It returns the action to perform, applying
// navigation and selection changes to the cursor and context itself.
func (c *MenuCursor) Activate(ctx *GameContext) MenuAction {
	screen := c.Screen()
	index := c.selection[screen]

	switch screen {
	case ScreenTrack:
		if ctx != nil && index < len(TrackMenuEntries) {
			ctx.MenuTrackIndex = uint8(index)
			ctx.TrackID = TrackMenuEntries[index].TrackID
		}
		c.Back()
		return ActionNone
	case ScreenClass:
		if ctx != nil {
			ctx.SpeedClass = SpeedClass(index)
		}
		c.Back()
		return ActionNone
	}

	items := Screens[screen].Items
	if index >= len(items) {
		return ActionNone
	}
	item := items[index]
	if item.Target != ScreenNone {
		c.stack = append(c.stack, item.Target)
		return ActionNone
	}
	return item.Action
}

// Back unwinds one screen. It reports false when already at the main screen, which
// is where retail leaves the front end.
func (c *MenuCursor) Back() bool {
	if len(c.stack) <= 1 {
		return false
	}
	c.stack = c.stack[:len(c.stack)-1]
	return true
}
