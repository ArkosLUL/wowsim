package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

const (
	talentTreesDir = "ui/core/talents/trees"
	simDBJSON      = "assets/database/db.json"
	serverHunter   = 3
)

// recordedSetup is a recorded run's .setup.txt, as mod-sim-validation's TestRecordedRun writes it.
// TestRecordedRunHunter adds the glyphs, bags, ammo and pet.
type recordedSetup struct {
	Player  string
	Class   int32
	Race    int32
	Seconds float64
	Yards   float64
	Talents []int32             // talent spells of the first talent group
	Items   map[int32]setupItem // by AzerothCore equipment slot
	Glyphs  [6]int32            // GlyphProperties ids
	Bags    []setupBag
	Ammo    int32
	Pet     *setupPet // nil when the run had no pet
}

type setupItem struct {
	ID           int32
	Enchantments string // item_instance.enchantments
}

type setupBag struct {
	Slot, Item, Class int32
}

type setupPet struct {
	Entry, Family int32
	Name          string
	Spells        []int32 // pet_spell rows: abilities and talents alike
}

func readSetup(path string) (*recordedSetup, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	setup, err := parseSetup(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return setup, nil
}

func parseSetup(r io.Reader) (*recordedSetup, error) {
	setup := &recordedSetup{Items: map[int32]setupItem{}}
	section := ""
	scanner := bufio.NewScanner(r)
	// each server snapshot is one long JSON line
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var err error
		if strings.HasPrefix(line, "  ") {
			err = setup.addRow(section, strings.Fields(line))
		} else {
			section, err = setup.addHeader(line)
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if setup.Class == 0 || setup.Race == 0 {
		return nil, fmt.Errorf("no class or race line")
	}
	return setup, nil
}

// addHeader reads a `key: value` line, or a section heading like `talents (spell, specMask):`, and
// returns the section the indented rows below it belong to.
func (setup *recordedSetup) addHeader(line string) (string, error) {
	key, value, _ := strings.Cut(line, ":")
	name, _, _ := strings.Cut(key, " (")
	value = strings.TrimSpace(value)

	var err error
	switch name {
	case "player":
		setup.Player = value
	case "class":
		setup.Class, err = parseInt32(value)
	case "race":
		setup.Race, err = parseInt32(value)
	case "seconds":
		setup.Seconds, err = strconv.ParseFloat(value, 64)
	case "yards":
		setup.Yards, err = strconv.ParseFloat(value, 64)
	case "ammo":
		setup.Ammo, err = parseInt32(value)
	case "glyphs":
		fields := strings.Fields(strings.Trim(value, "[]"))
		if len(fields) != len(setup.Glyphs) {
			return "", fmt.Errorf("%d glyphs, want %d", len(fields), len(setup.Glyphs))
		}
		for i, field := range fields {
			if setup.Glyphs[i], err = parseInt32(field); err != nil {
				break
			}
		}
	case "pet":
		setup.Pet, err = parsePet(value)
	case "talents", "equipped", "bags", "pet spells":
		return name, nil
	}
	// anything else, like the server snapshots, has nothing the sim takes
	return "", err
}

// parsePet reads `entry 28011 family 35 level 80 name Serpent`, or `none (why)`.
func parsePet(value string) (*setupPet, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 || fields[0] == "none" {
		return nil, nil
	}
	pet := &setupPet{}
	for i := 0; i+1 < len(fields); i += 2 {
		var err error
		switch fields[i] {
		case "entry":
			pet.Entry, err = parseInt32(fields[i+1])
		case "family":
			pet.Family, err = parseInt32(fields[i+1])
		case "name":
			pet.Name = strings.Join(fields[i+1:], " ")
			return pet, nil
		}
		if err != nil {
			return nil, err
		}
	}
	return pet, nil
}

func (setup *recordedSetup) addRow(section string, fields []string) error {
	if section == "" {
		return nil
	}
	if len(fields) < 2 {
		return fmt.Errorf("short %s row %q", section, strings.Join(fields, " "))
	}
	values := make([]int32, 0, 3)
	for _, field := range fields[:min(len(fields), 3)] {
		value, err := parseInt32(field)
		if err != nil {
			return err
		}
		values = append(values, value)
	}
	switch section {
	case "talents":
		// the specMask bit for talent group 0; the setup's glyphs are group 0's too
		if values[1]&1 != 0 {
			setup.Talents = append(setup.Talents, values[0])
		}
	case "equipped":
		setup.Items[values[0]] = setupItem{ID: values[1], Enchantments: strings.Join(fields[2:], " ")}
	case "bags":
		if len(values) < 3 {
			return fmt.Errorf("bag row %q has no item class", strings.Join(fields, " "))
		}
		setup.Bags = append(setup.Bags, setupBag{Slot: values[0], Item: values[1], Class: values[2]})
	case "pet spells":
		if setup.Pet == nil {
			return fmt.Errorf("pet spells without a pet line")
		}
		setup.Pet.Spells = append(setup.Pet.Spells, values[0])
	}
	return nil
}

func parseInt32(text string) (int32, error) {
	value, err := strconv.ParseInt(text, 10, 32)
	return int32(value), err
}

var acSlots = map[int32]proto.ItemSlot{
	0: proto.ItemSlot_ItemSlotHead, 1: proto.ItemSlot_ItemSlotNeck, 2: proto.ItemSlot_ItemSlotShoulder,
	4: proto.ItemSlot_ItemSlotChest, 5: proto.ItemSlot_ItemSlotWaist, 6: proto.ItemSlot_ItemSlotLegs,
	7: proto.ItemSlot_ItemSlotFeet, 8: proto.ItemSlot_ItemSlotWrist, 9: proto.ItemSlot_ItemSlotHands,
	10: proto.ItemSlot_ItemSlotFinger1, 11: proto.ItemSlot_ItemSlotFinger2, 12: proto.ItemSlot_ItemSlotTrinket1,
	13: proto.ItemSlot_ItemSlotTrinket2, 14: proto.ItemSlot_ItemSlotBack, 15: proto.ItemSlot_ItemSlotMainHand,
	16: proto.ItemSlot_ItemSlotOffHand, 17: proto.ItemSlot_ItemSlotRanged,
}

// petFamilies maps CreatureFamily.dbc ids to the sim's pets.
var petFamilies = map[int32]proto.Hunter_Options_PetType{
	1: proto.Hunter_Options_Wolf, 2: proto.Hunter_Options_Cat, 3: proto.Hunter_Options_Spider, 4: proto.Hunter_Options_Bear,
	5: proto.Hunter_Options_Boar, 6: proto.Hunter_Options_Crocolisk, 7: proto.Hunter_Options_CarrionBird,
	8: proto.Hunter_Options_Crab, 9: proto.Hunter_Options_Gorilla, 11: proto.Hunter_Options_Raptor,
	12: proto.Hunter_Options_Tallstrider, 20: proto.Hunter_Options_Scorpid, 21: proto.Hunter_Options_Turtle,
	24: proto.Hunter_Options_Bat, 25: proto.Hunter_Options_Hyena, 26: proto.Hunter_Options_BirdOfPrey,
	27: proto.Hunter_Options_WindSerpent, 30: proto.Hunter_Options_Dragonhawk, 31: proto.Hunter_Options_Ravager,
	32: proto.Hunter_Options_WarpStalker, 33: proto.Hunter_Options_SporeBat, 34: proto.Hunter_Options_NetherRay,
	35: proto.Hunter_Options_Serpent, 37: proto.Hunter_Options_Moth, 38: proto.Hunter_Options_Chimaera,
	39: proto.Hunter_Options_Devilsaur, 41: proto.Hunter_Options_Silithid, 42: proto.Hunter_Options_Worm,
	43: proto.Hunter_Options_Rhino, 44: proto.Hunter_Options_Wasp, 45: proto.Hunter_Options_CoreHound,
	46: proto.Hunter_Options_SpiritBeast,
}

// ammoTypes are the ammo items the sim has a setting for. Shatter Rounds (52020) is the bullet that
// matches Iceblade Arrow's 91.5 DPS.
var ammoTypes = map[int32]proto.Hunter_Options_Ammo{
	0:     proto.Hunter_Options_AmmoNone,
	52021: proto.Hunter_Options_IcebladeArrow, 52020: proto.Hunter_Options_IcebladeArrow,
	41165: proto.Hunter_Options_SaroniteRazorheads, 41586: proto.Hunter_Options_TerrorshaftArrow,
	31737: proto.Hunter_Options_TimelessArrow, 34581: proto.Hunter_Options_MysteriousArrow,
	33803: proto.Hunter_Options_AdamantiteStinger, 28056: proto.Hunter_Options_BlackflightArrow,
}

// fifteenPercentQuivers are the Nerubian Reinforced Quiver and its ammo pouch, which
// TestRecordedRunHunter equips.
var fifteenPercentQuivers = []int32{44448, 44447}

const (
	bagClassQuiver = 11
	// the dummy has no mana to drain and the run cheats power, so the sim mustn't run dry either
	unlimitedMP5 = 100000
)

// hunterRotation is TestRecordedRunHunter's rotation: Aspect of the Dragonhawk up, Rapid Fire,
// Kill Command and Bestial Wrath on cooldown, then Serpent Sting kept up, Arcane Shot on cooldown,
// else Steady Shot. Hunter's Mark comes from the options, since the run casts it before the pull.
const hunterRotation = `{"type":"TypeAPL","priorityList":[
 {"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":61847}}}}},"castSpell":{"spellId":{"spellId":61847}}}},
 {"action":{"castSpell":{"spellId":{"spellId":3045}}}},
 {"action":{"castSpell":{"spellId":{"spellId":34026}}}},
 {"action":{"castSpell":{"spellId":{"spellId":19574}}}},
 {"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":49001}}}}},"castSpell":{"spellId":{"spellId":49001}}}},
 {"action":{"castSpell":{"spellId":{"spellId":49045}}}},
 {"action":{"castSpell":{"spellId":{"spellId":49052}}}}
]}`

// rrsimData is what the conversion reads besides the setup: the live DBCs and the sim's own files.
type rrsimData struct {
	root       string
	dbc        *azerothcore.RosterDBC
	trees      azerothcore.TalentTrees
	glyphItems map[int32]int32 // glyph spell -> glyph item
}

func loadRRSimData(root, dbcDir string) (*rrsimData, error) {
	dbc, err := azerothcore.LoadRosterDBC(dbcDir)
	if err != nil {
		return nil, err
	}
	trees, err := azerothcore.LoadTalentTrees(filepath.Join(root, talentTreesDir))
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, simDBJSON))
	if err != nil {
		return nil, err
	}
	var db struct {
		GlyphIds []struct {
			ItemID  int32 `json:"itemId"`
			SpellID int32 `json:"spellId"`
		} `json:"glyphIds"`
	}
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("%s: %w", simDBJSON, err)
	}
	glyphItems := make(map[int32]int32, len(db.GlyphIds))
	for _, glyph := range db.GlyphIds {
		glyphItems[glyph.SpellID] = glyph.ItemID
	}
	return &rrsimData{root: root, dbc: dbc, trees: trees, glyphItems: glyphItems}, nil
}

type rrsimOptions struct {
	Seconds    float64
	Iterations int32
	NoTracking bool
}

// recordedHunter builds the sim player for a hunter's recorded run. It returns what it couldn't
// carry over as warnings, and fails on what would make the comparison meaningless.
func recordedHunter(setup *recordedSetup, data *rrsimData, noTracking bool) (*proto.Player, []string, error) {
	if setup.Class != serverHunter {
		return nil, nil, fmt.Errorf("class %d: only hunter runs have a rotation here", setup.Class)
	}
	race, ok := races[uint8(setup.Race)]
	if !ok {
		return nil, nil, fmt.Errorf("no sim race for server race %d", setup.Race)
	}
	var warnings []string

	talentSpells := setup.Talents
	if noTracking {
		tracking, err := talentRankSpells(filepath.Join(data.root, talentTreesDir, "hunter.json"), "improvedTracking")
		if err != nil {
			return nil, nil, err
		}
		talentSpells = slices.DeleteFunc(slices.Clone(talentSpells), func(spell int32) bool {
			return slices.Contains(tracking, spell)
		})
	}
	talents, _, unmatched := azerothcore.BuildTalentString(talentSpells, setup.Class, data.dbc, data.trees)
	for _, spell := range unmatched {
		warnings = append(warnings, fmt.Sprintf("talent spell %d isn't in the sim's talent trees", spell))
	}

	equipment, itemWarnings, err := recordedEquipment(setup.Items, data.dbc)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, itemWarnings...)

	major, minor, unknown := azerothcore.SplitGlyphs(setup.Glyphs, data.dbc)
	for _, glyph := range unknown {
		warnings = append(warnings, fmt.Sprintf("glyph %d isn't in GlyphProperties.dbc", glyph))
	}
	glyphs := &proto.Glyphs{}
	for i, slot := range []*int32{&glyphs.Major1, &glyphs.Major2, &glyphs.Major3} {
		if i < len(major) {
			*slot = data.glyphItem(major[i], &warnings)
		}
	}
	for i, slot := range []*int32{&glyphs.Minor1, &glyphs.Minor2, &glyphs.Minor3} {
		if i < len(minor) {
			*slot = data.glyphItem(minor[i], &warnings)
		}
	}

	options, err := hunterOptions(setup, data)
	if err != nil {
		return nil, nil, err
	}

	rotation := &proto.APLRotation{}
	if err := protojson.Unmarshal([]byte(hunterRotation), rotation); err != nil {
		return nil, nil, err
	}

	return &proto.Player{
		Name:               setup.Player,
		Race:               race,
		Class:              proto.Class_ClassHunter,
		Equipment:          equipment,
		TalentsString:      talents,
		Glyphs:             glyphs,
		Spec:               &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}},
		Rotation:           rotation,
		DistanceFromTarget: setup.Yards,
		Consumes:           &proto.Consumes{},
		Buffs:              &proto.IndividualBuffs{},
		BonusStats:         &proto.UnitStats{Stats: stats.Stats{stats.MP5: unlimitedMP5}.ToFloatArray()},
	}, warnings, nil
}

func (data *rrsimData) glyphItem(spell int32, warnings *[]string) int32 {
	item, ok := data.glyphItems[spell]
	if !ok {
		*warnings = append(*warnings, fmt.Sprintf("glyph spell %d has no glyph item in the sim", spell))
	}
	return item
}

func recordedEquipment(items map[int32]setupItem, dbc *azerothcore.RosterDBC) (*proto.EquipmentSpec, []string, error) {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, len(proto.ItemSlot_name))}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	var warnings []string
	for acSlot, item := range items {
		slot, ok := acSlots[acSlot]
		if !ok {
			continue // shirt and tabard
		}
		simItem, ok := core.LookupItem(item.ID)
		if !ok {
			return nil, nil, fmt.Errorf("slot %d: item %d isn't in the sim's item database", acSlot, item.ID)
		}
		built, itemWarnings := azerothcore.BuildRosterItem(azerothcore.EquippedItem{
			ACSlot: acSlot, ItemID: item.ID, Enchantments: item.Enchantments,
			NativeSockets: int32(len(simItem.GemSockets)), KnownTemplate: true,
		}, dbc, nil)
		warnings = append(warnings, itemWarnings...)
		gems := built.Gems
		if built.ExtraGem != 0 {
			gems = append(slices.Clone(gems), built.ExtraGem)
		}
		equipment.Items[slot] = &proto.ItemSpec{Id: built.ID, Enchant: built.Enchant, Gems: gems}
	}
	return equipment, warnings, nil
}

func hunterOptions(setup *recordedSetup, data *rrsimData) (*proto.Hunter_Options, error) {
	ammo, ok := ammoTypes[setup.Ammo]
	if !ok {
		return nil, fmt.Errorf("ammo %d has no sim setting", setup.Ammo)
	}
	quiver := proto.Hunter_Options_QuiverNone
	for _, bag := range setup.Bags {
		if bag.Class != bagClassQuiver {
			continue
		}
		if !slices.Contains(fifteenPercentQuivers, bag.Item) {
			return nil, fmt.Errorf("quiver %d isn't one the sim knows the haste of", bag.Item)
		}
		quiver = proto.Hunter_Options_Quiver15Percent
	}
	options := &proto.Hunter_Options{
		Ammo:                 ammo,
		Quiver:               quiver,
		PetUptime:            1,
		SniperTrainingUptime: 1, // the run never moves
		UseHuntersMark:       true,
	}
	if setup.Pet == nil {
		return options, nil
	}
	petType, ok := petFamilies[setup.Pet.Family]
	if !ok {
		return nil, fmt.Errorf("pet family %d has no sim pet", setup.Pet.Family)
	}
	talents, err := petTalents(filepath.Join(data.root, talentTreesDir), setup.Pet.Spells)
	if err != nil {
		return nil, err
	}
	options.PetType = petType
	options.PetTalents = talents
	return options, nil
}

type talentTreeFile []struct {
	Talents []struct {
		FieldName string  `json:"fieldName"`
		MaxPoints int32   `json:"maxPoints"`
		SpellIds  []int32 `json:"spellIds"`
	} `json:"talents"`
}

func readTalentTreeFile(path string) (talentTreeFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file talentTreeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

// rankSpells is a talent's spell per rank. The tree files leave out some ranks' spells, which follow
// on from the last listed id.
func rankSpells(listed []int32, maxPoints int32) []int32 {
	spells := make([]int32, maxPoints)
	for i := range spells {
		if i < len(listed) {
			spells[i] = listed[i]
		} else {
			spells[i] = listed[len(listed)-1] + int32(i-len(listed)+1)
		}
	}
	return spells
}

func talentRankSpells(path, fieldName string) ([]int32, error) {
	file, err := readTalentTreeFile(path)
	if err != nil {
		return nil, err
	}
	for _, tree := range file {
		for _, talent := range tree.Talents {
			if talent.FieldName == fieldName {
				return rankSpells(talent.SpellIds, talent.MaxPoints), nil
			}
		}
	}
	return nil, fmt.Errorf("%s: no talent %s", path, fieldName)
}

// petTalents reads the pet's talents off its spells, against the sim's three pet trees. The trees
// share talents, like Cobra Reflexes, so a pet's spells can match in more than one.
func petTalents(treesDir string, spells []int32) (*proto.HunterPetTalents, error) {
	talents := &proto.HunterPetTalents{}
	msg := talents.ProtoReflect()
	for _, tree := range []string{"hunter_cunning", "hunter_ferocity", "hunter_tenacity"} {
		file, err := readTalentTreeFile(filepath.Join(treesDir, tree+".json"))
		if err != nil {
			return nil, err
		}
		for _, talent := range file[0].Talents {
			ranks := rankSpells(talent.SpellIds, talent.MaxPoints)
			rank := int32(0)
			for i, spell := range ranks {
				if slices.Contains(spells, spell) {
					rank = int32(i + 1)
				}
			}
			if rank == 0 {
				continue
			}
			fd := msg.Descriptor().Fields().ByJSONName(talent.FieldName)
			if fd == nil {
				return nil, fmt.Errorf("%s: no pet talent field %s", tree, talent.FieldName)
			}
			if fd.Kind() == protoreflect.BoolKind {
				msg.Set(fd, protoreflect.ValueOfBool(true))
			} else {
				msg.Set(fd, protoreflect.ValueOfInt32(rank))
			}
		}
	}
	return talents, nil
}

func recordedRunRequest(player *proto.Player, opts rrsimOptions) *proto.RaidSimRequest {
	// the boss dummy is a giant; the default target's other stats already match it
	target := googleProto.Clone(core.NewDefaultTarget()).(*proto.Target)
	target.MobType = proto.MobType_MobTypeGiant
	return &proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  &proto.Encounter{Duration: opts.Seconds, Targets: []*proto.Target{target}},
		SimOptions: &proto.SimOptions{Iterations: opts.Iterations, RandomSeed: 101},
	}
}

func runRecordedSim(args []string) int {
	flags := flag.NewFlagSet("simval rrsim", flag.ExitOnError)
	seconds := flags.Float64("seconds", 0, "fight length; 0 takes the setup's. Pass the active duration `simval chronicle` reports")
	iterations := flags.Int("iterations", 3000, "sim iterations")
	dbcDir := flags.String("dbc", "/dbc", "the live server's DBC files")
	noTracking := flags.Bool("notracking", false, "drop Improved Tracking, for a run that tracked nothing")
	out := flags.String("out", "", "also write the sim request here as JSON")
	verbose := flags.Bool("v", false, "also print the final stats, the talents and an ability breakdown")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: simval rrsim [flags] <setup file>\n\n"+
			"Sims a recorded hunter run from its .setup.txt, for `simval chronicle -sim`. Captured runs\n"+
			"live in %s. Run from the repo root, built with --tags=with_db.\n\n", chronicleDir)
		flags.PrintDefaults()
	}
	_ = flags.Parse(args)
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	if !core.WITH_DB {
		fmt.Fprintln(os.Stderr, "the sim has no item database: go run --tags=with_db ./tools/simval rrsim <setup file>")
		return 1
	}

	setupPath := flags.Arg(0)
	setup, err := readSetup(setupPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	opts := rrsimOptions{Seconds: *seconds, Iterations: int32(*iterations), NoTracking: *noTracking}
	if opts.Seconds == 0 {
		opts.Seconds = setup.Seconds
	}

	sim.RegisterAll()
	data, err := loadRRSimData(".", *dbcDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	player, warnings, err := recordedHunter(setup, data, opts.NoTracking)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	request := recordedRunRequest(player, opts)
	if *out != "" {
		js, err := protojson.MarshalOptions{Multiline: true}.Marshal(request)
		if err == nil {
			err = os.WriteFile(*out, js, 0o644)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	result := core.RunRaidSim(request)
	if result.ErrorResult != "" {
		fmt.Fprintln(os.Stderr, result.ErrorResult)
		return 1
	}
	reportRecordedSim(os.Stdout, setup, request, result, *verbose)
	if log := capturePath(setupPath); log != "" {
		fmt.Printf("compare: simval chronicle -sim %.1f %s\n", result.RaidMetrics.Parties[0].Players[0].Dps.Avg, log)
	}
	return 0
}

func reportRecordedSim(out io.Writer, setup *recordedSetup, request *proto.RaidSimRequest, result *proto.RaidSimResult, verbose bool) {
	player := request.Raid.Parties[0].Players[0]
	options := player.GetHunter().Options
	fmt.Fprintf(out, "%s, %s hunter: talents %s, pet %v\n", setup.Player, azerothcore.RaceNames[setup.Race],
		player.TalentsString, options.PetType)

	if verbose {
		fmt.Fprintf(out, "pet talents {%v}\nglyphs {%v}\n", options.PetTalents, player.Glyphs)
		computed := core.ComputeStats(&proto.ComputeStatsRequest{Raid: request.Raid, Encounter: request.Encounter})
		final := computed.RaidStats.Parties[0].Players[0].FinalStats.Stats
		fmt.Fprintf(out, "final: RAP %.0f, agi %.0f, crit rating %.0f, hit rating %.0f, haste rating %.0f, ArP %.0f\n",
			final[proto.Stat_StatRangedAttackPower], final[proto.Stat_StatAgility], final[proto.Stat_StatMeleeCrit],
			final[proto.Stat_StatMeleeHit], final[proto.Stat_StatMeleeHaste], final[proto.Stat_StatArmorPenetration])
	}

	metrics := result.RaidMetrics.Parties[0].Players[0]
	petDPS := 0.0
	for _, pet := range metrics.Pets {
		petDPS += pet.Dps.Avg
	}
	// the player's DPS counts its pet's, as chronicle's does
	fmt.Fprintf(out, "sim: %.1f DPS, %.1f of it the pet's, stdev %.1f, %d iterations of %.1f s\n",
		metrics.Dps.Avg, petDPS, metrics.Dps.Stdev, request.SimOptions.Iterations, request.Encounter.Duration)

	if verbose {
		iterations := float64(request.SimOptions.Iterations)
		printActions(out, "", metrics.Actions, iterations)
		for _, pet := range metrics.Pets {
			printActions(out, pet.Name+": ", pet.Actions, iterations)
		}
	}
}

func printActions(out io.Writer, prefix string, actions []*proto.ActionMetrics, iterations float64) {
	for _, action := range actions {
		var damage, casts, hits, crits float64
		for _, target := range action.Targets {
			damage += target.Damage
			casts += float64(target.Casts)
			hits += float64(target.Hits + target.Crits)
			crits += float64(target.Crits)
		}
		if damage == 0 {
			continue
		}
		fmt.Fprintf(out, "  %-40s %9.0f dmg %6.1f casts %6.1f hits %5.1f%% crit  per iteration\n",
			prefix+action.Id.String(), damage/iterations, casts/iterations, hits/iterations, 100*crits/max(hits, 1))
	}
}

// capturePath is the capture next to a setup file, or "" when there's none.
func capturePath(setupPath string) string {
	base := strings.TrimSuffix(setupPath, ".setup.txt")
	for _, ext := range []string{".log.gz", ".log"} {
		if _, err := os.Stat(base + ext); err == nil {
			return base + ext
		}
	}
	return ""
}
