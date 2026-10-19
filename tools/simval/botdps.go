package main

import (
	"bufio"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// botdpsFile is mod-sim-validation's TestBotDps input: each build's character as the sim plays it at one
// phase, with the server's ids worked out, plus the page settings `botdps compare` sims it from.
type botdpsFile struct {
	Phase  int           `json:"phase"`
	Setups []botdpsSetup `json:"setups"`
}

type botdpsSetup struct {
	// the build's key in presetgen's BUILDS
	Name  string `json:"name"`
	Class uint8  `json:"class"`
	Race  uint8  `json:"race"`
	// the race whose racials mod-racial-trait-swap gives the character, 0 for its own
	Traits       uint8             `json:"traits,omitempty"`
	Items        []goldcharItem    `json:"items"`
	Enchants     []goldcharEnchant `json:"enchants,omitempty"`
	Gems         []goldcharGem     `json:"gems,omitempty"`
	Reforges     []botdpsReforge   `json:"reforges,omitempty"`
	Talents      string            `json:"talents"`
	TalentSpells []int32           `json:"talentSpells"`
	Glyphs       []goldcharGlyph   `json:"glyphs"`
	// the combat potion the Playerbot switch drinks, 0 for none
	Potion int32 `json:"potion,omitempty"`
	// a mana caster, whom the e2e blesses with Wisdom rather than Might: one paladin's blessings don't stack
	Caster bool `json:"caster,omitempty"`
	// a hunter's ammo item for the ranged weapon's kind, and its pet's CreatureFamily.dbc name for the bot's
	// `tame family`
	Ammo      int32  `json:"ammo,omitempty"`
	PetFamily string `json:"petFamily,omitempty"`
	// the SkillLine ids of the professions the sim gives the player, whose passives (Master of Anatomy,
	// Toughness, Mixology) come with the skill at 450
	Skills []int32 `json:"skills,omitempty"`
	// base64 of the captured IndividualSimSettings
	Settings string `json:"settings"`
}

// botdpsReforge is mod-reforging's reforge of the item in a server slot, by ItemModType.
type botdpsReforge struct {
	Slot uint8 `json:"slot"`
	From int32 `json:"from"`
	To   int32 `json:"to"`
}

// botdpsPotionItems are the potions playerbotPotion picks, as items.
var botdpsPotionItems = map[proto.Potions]int32{
	proto.Potions_PotionOfSpeed:     40211,
	proto.Potions_PotionOfWildMagic: 40212,
}

// botdpsAmmoItems are each ammo setting's arrow and bullet: the sim's setting is named for the arrow, and
// the bullet of the same tier has its damage.
var botdpsAmmoItems = map[proto.Hunter_Options_Ammo][2]int32{
	proto.Hunter_Options_IcebladeArrow:      {52021, 52020},
	proto.Hunter_Options_SaroniteRazorheads: {41165, 41164},
	proto.Hunter_Options_TerrorshaftArrow:   {41586, 41584},
}

var botdpsProfessionSkills = map[proto.Profession]int32{
	proto.Profession_Alchemy: 171, proto.Profession_Blacksmithing: 164, proto.Profession_Enchanting: 333,
	proto.Profession_Engineering: 202, proto.Profession_Herbalism: 182, proto.Profession_Inscription: 773,
	proto.Profession_Jewelcrafting: 755, proto.Profession_Leatherworking: 165, proto.Profession_Mining: 186,
	proto.Profession_Skinning: 393, proto.Profession_Tailoring: 197,
}

// botdpsServerIDs are the server's ids botdpsSetupOf looks a build's glyphs and pet up in.
type botdpsServerIDs struct {
	// glyph spell -> GlyphProperties id; a glyph the server lacks gets 0, which the e2e leaves out
	glyphs map[int32]int32
	// glyph item -> glyph spell
	glyphSpells map[int32]int32
	// the sim's pet -> its CreatureFamily.dbc name
	petFamilies map[proto.Hunter_Options_PetType]string
}

// botdpsDummy is the boss dummy (999000), which never drops in health: the dummy's AI zeroes every hit, so
// no execute phase comes.
func botdpsDummy() *proto.Target {
	target := googleProto.Clone(core.NewDefaultTarget()).(*proto.Target)
	target.MobType = mobTypes[bossDummy.CreatureType]
	worldBoss := bossDummy.WorldBoss
	target.WorldBoss = &worldBoss
	target.NoXp = true
	target.MechanicImmuneMask = dummyMechanicImmunities(dummyBossEntry)
	return target
}

const dummyBossEntry = 999000

func runBotDps(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "setups":
			return runBotDpsSetups(args[1:])
		case "compare":
			return runBotDpsCompare(args[1:])
		}
	}
	fmt.Fprintln(os.Stderr, "usage: simval botdps setups|compare [flags] ...")
	return 2
}

func runBotDpsSetups(args []string) int {
	flags := flag.NewFlagSet("simval botdps setups", flag.ExitOnError)
	dir := flags.String("settings", "", "a capture's settings/ directory (statweights' capture.py)")
	phase := flags.Int("phase", 3, "the content phase whose captured settings to read")
	builds := flags.String("builds", "", "comma-separated build keys; empty takes every one captured at the phase")
	dbcDir := flags.String("dbc", "/dbc", "the live server's DBC files, for the GlyphProperties ids")
	out := flags.String("out", "", "where to write the setups file")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: simval botdps setups -settings <dir> -out <file> [flags]\n\n"+
			"Writes mod-sim-validation's TestBotDps setups from captured page settings. Run from the repo root,\n"+
			"built with --tags=with_db.\n\n")
		flags.PrintDefaults()
	}
	_ = flags.Parse(args)
	if *dir == "" || *out == "" || flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	if !core.WITH_DB {
		fmt.Fprintln(os.Stderr, "the sim has no item database: go run --tags=with_db ./tools/simval botdps setups ...")
		return 1
	}
	sim.RegisterAll()

	keys, err := botdpsBuildKeys(*dir, *phase, *builds)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dbc, err := azerothcore.LoadRosterDBC(*dbcDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ids := botdpsServerIDs{glyphs: map[int32]int32{}, petFamilies: map[proto.Hunter_Options_PetType]string{}}
	for id, glyph := range dbc.Glyphs {
		ids.glyphs[glyph.SpellID] = id
	}
	for id, pet := range petFamilies {
		ids.petFamilies[pet] = dbc.Families[id].Name
	}
	if ids.glyphSpells, err = glyphItemSpells(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	file := botdpsFile{Phase: *phase}
	for _, key := range keys {
		raw, err := os.ReadFile(filepath.Join(*dir, fmt.Sprintf("%s_p%d.bin", key, *phase)))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		settings, err := botdpsLoadSettings(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", key, err)
			return 1
		}
		setup, warnings, err := botdpsSetupOf(".", key, settings, ids)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", key, err)
			return 1
		}
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s: %s\n", key, warning)
		}
		file.Setups = append(file.Setups, setup)
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err == nil {
		err = os.WriteFile(*out, append(data, '\n'), 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("wrote %d setups to %s\n", len(file.Setups), *out)
	return 0
}

// botdpsLoadSettings reads a captured settings file: the ComputeStatsRequest the page sent, as the rankings
// runner reads it.
func botdpsLoadSettings(data []byte) (*proto.IndividualSimSettings, error) {
	req := &proto.ComputeStatsRequest{}
	if err := googleProto.Unmarshal(data, req); err != nil {
		return nil, err
	}
	if len(req.GetRaid().GetParties()) == 0 || len(req.Raid.Parties[0].Players) == 0 {
		return nil, fmt.Errorf("no player in the request")
	}
	party := req.Raid.Parties[0]
	return &proto.IndividualSimSettings{
		Player:     party.Players[0],
		PartyBuffs: party.Buffs,
		RaidBuffs:  req.Raid.Buffs,
		Debuffs:    req.Raid.Debuffs,
		Encounter:  req.Encounter,
	}, nil
}

// botdpsBuildKeys are the listed builds, or every build with a settings file at the phase.
func botdpsBuildKeys(dir string, phase int, list string) ([]string, error) {
	if list != "" {
		return strings.Split(list, ","), nil
	}
	suffix := fmt.Sprintf("_p%d.bin", phase)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, entry := range entries {
		if key, ok := strings.CutSuffix(entry.Name(), suffix); ok {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%s holds no *%s", dir, suffix)
	}
	return keys, nil
}

// botdpsSetupOf works out the server's side of a build's captured settings.
func botdpsSetupOf(root, key string, settings *proto.IndividualSimSettings, ids botdpsServerIDs) (botdpsSetup,
	[]string, error) {
	player := settings.GetPlayer()
	setup := goldcharSetup{Talents: player.GetTalentsString()}
	for id, c := range classes {
		if c.class == player.GetClass() {
			setup.Class = id
		}
	}
	for id, race := range races {
		if race == player.GetRace() {
			setup.Race = id
		}
	}
	if setup.Class == 0 || setup.Race == 0 {
		return botdpsSetup{}, nil, fmt.Errorf("no server ids for %s %s", player.GetRace(), player.GetClass())
	}
	var traits uint8
	if t := player.GetRacialTraits(); t != proto.Race_RaceUnknown && t != player.GetRace() {
		for id, race := range races {
			if race == t {
				traits = id
			}
		}
		if traits == 0 {
			return botdpsSetup{}, nil, fmt.Errorf("no server id for the racial traits %s", t)
		}
	}

	var warnings []string
	var items []gearSetItem
	var reforges []botdpsReforge
	for i, item := range player.GetEquipment().GetItems() {
		items = append(items, gearSetItem{ID: item.GetId(), Enchant: item.GetEnchant(), Gems: item.GetGems()})
		r := item.GetReforge()
		if r != nil && r.FromStatType != 0 && r.ToStatType != 0 && item.GetId() != 0 && i < len(serverEquipSlots) {
			reforges = append(reforges, botdpsReforge{Slot: serverEquipSlots[i], From: r.FromStatType, To: r.ToStatType})
		}
	}
	if err := setup.fill(root, key, items, player.GetGlyphs(), ids.glyphSpells); err != nil {
		return botdpsSetup{}, nil, err
	}
	for i, g := range setup.Glyphs {
		setup.Glyphs[i].Glyph = ids.glyphs[g.Spell]
		if setup.Glyphs[i].Glyph == 0 {
			warnings = append(warnings, fmt.Sprintf("glyph spell %d isn't in GlyphProperties.dbc", g.Spell))
		}
	}

	raw, err := googleProto.Marshal(settings)
	if err != nil {
		return botdpsSetup{}, nil, err
	}
	out := botdpsSetup{
		Name: key, Class: setup.Class, Race: setup.Race, Traits: traits, Items: setup.Items, Enchants: setup.Enchants,
		Gems: setup.Gems, Reforges: reforges, Talents: setup.Talents, TalentSpells: setup.TalentSpells, Glyphs: setup.Glyphs,
		Settings: base64.StdEncoding.EncodeToString(raw),
	}

	potion, err := botdpsPotion(player)
	if err != nil {
		return botdpsSetup{}, nil, err
	}
	out.Potion = botdpsPotionItems[potion]
	for _, profession := range core.ProtoToProfessions(player) {
		skill, ok := botdpsProfessionSkills[profession]
		if !ok {
			return botdpsSetup{}, nil, fmt.Errorf("no skill line for %s", profession)
		}
		out.Skills = append(out.Skills, skill)
	}
	switch player.GetSpec().(type) {
	case *proto.Player_Mage, *proto.Player_Warlock, *proto.Player_ShadowPriest, *proto.Player_BalanceDruid,
		*proto.Player_ElementalShaman:
		out.Caster = true
	}
	if hunter := player.GetHunter(); hunter != nil {
		ammo, ok := botdpsAmmoItems[hunter.GetOptions().GetAmmo()]
		switch {
		case hunter.GetOptions().GetAmmo() == proto.Hunter_Options_AmmoNone:
		case !ok:
			return botdpsSetup{}, nil, fmt.Errorf("ammo %v has no items here", hunter.GetOptions().GetAmmo())
		default:
			var ranged int32
			if items := player.GetEquipment().GetItems(); int(proto.ItemSlot_ItemSlotRanged) < len(items) {
				ranged = items[proto.ItemSlot_ItemSlotRanged].GetId()
			}
			out.Ammo = ammo[0]
			if item, ok := core.LookupItem(ranged); ok && item.RangedWeaponType == proto.RangedWeaponType_RangedWeaponTypeGun {
				out.Ammo = ammo[1]
			}
		}
		if pet := hunter.GetOptions().GetPetType(); pet != proto.Hunter_Options_PetNone {
			out.PetFamily = ids.petFamilies[pet]
			if out.PetFamily == "" {
				return botdpsSetup{}, nil, fmt.Errorf("no creature family for the pet %s", pet)
			}
		}
	}
	return out, warnings, nil
}

// botdpsPotion is the combat potion the Playerbot switch gives the player.
func botdpsPotion(player *proto.Player) (potion proto.Potions, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("building the sim character: %v", r)
		}
	}()
	player = googleProto.Clone(player).(*proto.Player)
	player.Playerbot = true
	s := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{botdpsDummy()}},
		SimOptions: &proto.SimOptions{RandomSeed: 1, Iterations: 1},
	})
	return s.Raid.Parties[0].Players[0].GetCharacter().Consumes.GetDefaultPotion(), nil
}

// botdpsRun is what the e2e's .setup.txt says about one bot fight.
type botdpsRun struct {
	Build  string
	Player string
	// the pull on the log's clock, in unix ms, and the fight's length from it
	PullMs  int64
	Seconds float64
	// the buff and debuff switches the bot fought under
	Buffs []string
	// the ones gone again after the fight
	Lost []string
}

// readBotDpsRun reads the `key: value` lines TestBotDps writes next to its capture.
func readBotDpsRun(path string) (botdpsRun, error) {
	f, err := os.Open(path)
	if err != nil {
		return botdpsRun{}, err
	}
	defer f.Close()
	var run botdpsRun
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch name {
		case "build":
			run.Build = value
		case "player":
			run.Player = value
		case "pull":
			run.PullMs, _ = strconv.ParseInt(value, 10, 64)
		case "seconds":
			run.Seconds, _ = strconv.ParseFloat(value, 64)
		case "buffs":
			run.Buffs = strings.Fields(value)
		case "buffs lost":
			run.Lost = strings.Fields(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return run, err
	}
	if run.Build == "" || run.Player == "" || run.PullMs == 0 || run.Seconds <= 0 {
		return run, fmt.Errorf("%s lacks the build, player, pull or seconds", path)
	}
	return run, nil
}

// botdpsRequest is the build's captured settings as a bot plays them on the dummy: the Playerbot switch
// on, the buffs and debuffs the bot had, for the fight's length.
func botdpsRequest(settings *proto.IndividualSimSettings, buffs []string, seconds float64, iterations int32,
	seed int64) (*proto.RaidSimRequest, error) {
	set, err := buffSetOf(buffs)
	if err != nil {
		return nil, err
	}
	player := googleProto.Clone(settings.GetPlayer()).(*proto.Player)
	player.Playerbot = true
	player.Buffs = set.individual
	return &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(player, set.party, set.raid, set.debuffs),
		// no RaidDifficulty: mod-dungeon-scale leaves the dummy out, whatever raid the page sims
		Encounter: &proto.Encounter{
			Duration:       seconds,
			Targets:        []*proto.Target{botdpsDummy()},
			ServerSettings: settings.GetEncounter().GetServerSettings(),
		},
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: seed},
	}, nil
}

// botdpsResult is one bot fight next to the sim's.
type botdpsResult struct {
	Run     botdpsRun
	Seconds float64
	BotDPS  float64
	SimDPS  float64
	SimSD   float64
	// casts per spell id: the bot's SPELL_CAST_SUCCESS events, the sim's mean casts
	BotCasts map[int32]int
	SimCasts map[int32]float64
	Names    map[int32]string
	// target aura uptime in percent, by spell id
	BotUptime map[int32]float64
	SimUptime map[int32]float64
}

func (r botdpsResult) ratio() float64 { return 100 * r.BotDPS / r.SimDPS }

func runBotDpsCompare(args []string) int {
	flags := flag.NewFlagSet("simval botdps compare", flag.ExitOnError)
	setupsPath := flags.String("setups", "", "the setups file the e2e ran")
	iterations := flags.Int("iterations", 3000, "sim iterations")
	seed := flags.Int64("seed", 101, "the sim's random seed")
	verbose := flags.Bool("v", false, "also list every cast and aura, not only those where bot and sim differ")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: simval botdps compare -setups <file> [flags] <capture.log>...\n\n"+
			"Sims each TestBotDps capture's build as a bot plays it and reports the bot's DPS against the sim's,\n"+
			"with their casts and target aura uptimes. Run from the repo root, built with --tags=with_db.\n\n")
		flags.PrintDefaults()
	}
	_ = flags.Parse(args)
	if *setupsPath == "" || flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	if !core.WITH_DB {
		fmt.Fprintln(os.Stderr, "the sim has no item database: go run --tags=with_db ./tools/simval botdps compare ...")
		return 1
	}
	sim.RegisterAll()

	raw, err := os.ReadFile(*setupsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var file botdpsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *setupsPath, err)
		return 1
	}

	var results []botdpsResult
	failed := 0
	for _, path := range flags.Args() {
		result, err := compareBotDps(file, path, int32(*iterations), *seed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			failed++
			continue
		}
		reportBotDps(os.Stdout, result, *verbose)
		results = append(results, result)
	}
	if len(results) > 1 {
		reportBotDpsSummary(os.Stdout, results)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d of %d captures failed\n", failed, flags.NArg())
		return 1
	}
	return 0
}

func compareBotDps(file botdpsFile, path string, iterations int32, seed int64) (botdpsResult, error) {
	run, err := readBotDpsRun(strings.TrimSuffix(path, ".log") + ".setup.txt")
	if err != nil {
		return botdpsResult{}, err
	}
	i := slices.IndexFunc(file.Setups, func(s botdpsSetup) bool { return s.Name == run.Build })
	if i < 0 {
		return botdpsResult{}, fmt.Errorf("the setups file has no build %s", run.Build)
	}
	raw, err := base64.StdEncoding.DecodeString(file.Setups[i].Settings)
	if err != nil {
		return botdpsResult{}, err
	}
	settings := &proto.IndividualSimSettings{}
	if err := googleProto.Unmarshal(raw, settings); err != nil {
		return botdpsResult{}, err
	}

	log, err := readChronicleLog(path)
	if err != nil {
		return botdpsResult{}, err
	}
	result, err := measureBotDps(log, run)
	if err != nil {
		return botdpsResult{}, err
	}

	request, err := botdpsRequest(settings, run.Buffs, result.Seconds, iterations, seed)
	if err != nil {
		return botdpsResult{}, err
	}
	simResult := core.RunRaidSim(request)
	if simResult.ErrorResult != "" {
		return botdpsResult{}, fmt.Errorf("the sim: %s", simResult.ErrorResult)
	}
	player := simResult.RaidMetrics.Parties[0].Players[0]
	result.SimDPS, result.SimSD = player.Dps.Avg, player.Dps.Stdev
	result.SimCasts = botdpsSimCasts(player, iterations)
	for _, aura := range simResult.EncounterMetrics.GetTargets()[0].GetAuras() {
		if id := aura.GetId().GetSpellId(); id != 0 {
			result.SimUptime[id] = math.Min(100, 100*aura.UptimeSecondsAvg/result.Seconds)
		}
	}
	return result, nil
}

// measureBotDps reads the bot's side of a capture, from the pull for the fight's length: the damage it and
// its minions did to the dummy, their casts, and the dummy's auras they kept up.
func measureBotDps(log *chronicleLog, run botdpsRun) (botdpsResult, error) {
	stats, err := analyzeChronicle(log, chronicleOptions{Source: run.Player})
	if err != nil {
		return botdpsResult{}, err
	}
	pull, end := run.PullMs, run.PullMs+int64(run.Seconds*1000)
	if last := log.Events[len(log.Events)-1].TimeMs; last < end {
		return botdpsResult{}, fmt.Errorf("the log ends %.1f s into the fight: Chronicle hadn't written the rest",
			float64(last-pull)/1000)
	}
	mine := map[string]bool{stats.Source.GUID: true}
	for guid := range log.Owners {
		if ownerOf(log, guid) == stats.Source.GUID {
			mine[guid] = true
		}
	}
	during := func(e chronicleEvent) bool { return mine[e.Src.GUID] && e.TimeMs >= pull && e.TimeMs < end }
	// the dummy is the unit the player hurt most
	hurt := map[string]int64{}
	for _, e := range log.Events {
		if during(e) && e.isDamage() {
			hurt[e.Dst.GUID] += e.damage().Amount
		}
	}
	var target string
	for guid, amount := range hurt {
		if target == "" || amount > hurt[target] {
			target = guid
		}
	}
	if target == "" {
		return botdpsResult{}, fmt.Errorf("%s hit nothing in the fight", run.Player)
	}

	result := botdpsResult{Run: run, Seconds: run.Seconds, BotCasts: map[int32]int{}, Names: map[int32]string{},
		BotUptime: map[int32]float64{}, SimUptime: map[int32]float64{}}
	var damage int64
	upSince, upMs := map[int32]int64{}, map[int32]int64{}
	for _, e := range log.Events {
		switch {
		case during(e) && e.isDamage() && e.Dst.GUID == target:
			damage += e.damage().Amount
		case during(e) && e.Type == "SPELL_CAST_SUCCESS" && botdpsChannels[e.Spell.ID] == 0:
			result.BotCasts[e.Spell.ID]++
			result.Names[e.Spell.ID] = e.Spell.Name
		case mine[e.Src.GUID] && e.Dst.GUID == target && (e.Type == "SPELL_AURA_APPLIED" || e.Type == "SPELL_AURA_REMOVED"):
			// an aura put up before the pull counts from it
			at := min(max(e.TimeMs, pull), end)
			since, up := upSince[e.Spell.ID]
			if e.Type == "SPELL_AURA_APPLIED" && !up {
				upSince[e.Spell.ID] = at
			} else if e.Type == "SPELL_AURA_REMOVED" && up {
				upMs[e.Spell.ID] += at - since
				delete(upSince, e.Spell.ID)
			}
			result.Names[e.Spell.ID] = e.Spell.Name
		}
	}
	for id, since := range upSince {
		upMs[id] += end - since
	}
	for id, ms := range upMs {
		if ms > 0 {
			result.BotUptime[id] = 100 * float64(ms) / 1000 / run.Seconds
		}
	}
	result.BotDPS = float64(damage) / run.Seconds
	return result, nil
}

// botdpsChannels are channels the sim books under the spell each tick casts, tagged with the channel's
// length, one cast a channel. The log has the channel's own cast, and one of that spell a tick.
var botdpsChannels = map[int32]int32{
	58381: 48156, // Mind Flay
}

// botdpsSimCasts are the sim's mean casts per spell id, the player's and its pets'. A spell's casts are its
// lowest tag's: a higher tag is a part the cast sets off (Pyroblast's dot), which the server logs no cast for.
func botdpsSimCasts(player *proto.UnitMetrics, iterations int32) map[int32]float64 {
	casts := map[int32]float64{}
	tags := map[int32]int32{}
	add := func(actions []*proto.ActionMetrics) {
		for _, action := range actions {
			id, tag := action.GetId().GetSpellId(), action.GetId().GetTag()
			var n float64
			for _, t := range action.Targets {
				n += float64(t.Casts) / float64(iterations)
			}
			lowest, seen := tags[id]
			switch {
			case id == 0 || seen && tag > lowest:
			case botdpsChannels[id] != 0:
				casts[botdpsChannels[id]] += n
			case seen && tag < lowest:
				casts[id], tags[id] = n, tag
			default:
				casts[id] += n
				tags[id] = tag
			}
		}
	}
	add(player.Actions)
	for _, pet := range player.Pets {
		add(pet.Actions)
	}
	return casts
}

// botdpsCastsDiffer is whether a cast count strays far enough for the report to list it: by more than 10% and
// more than 2 casts.
func botdpsCastsDiffer(bot int, sim float64) bool {
	return math.Abs(float64(bot)-sim) > math.Max(2, 0.1*sim)
}

func reportBotDps(out io.Writer, r botdpsResult, verbose bool) {
	fmt.Fprintf(out, "%s (%s, %.1f s): bot %.0f DPS, sim %.0f ± %.0f, bot/sim %.1f%%\n", r.Run.Build, r.Run.Player,
		r.Seconds, r.BotDPS, r.SimDPS, r.SimSD, r.ratio())
	if len(r.Run.Lost) > 0 {
		fmt.Fprintf(out, "  buffs gone by the end, the sim keeps them: %s\n", strings.Join(r.Run.Lost, " "))
	}
	ids := map[int32]bool{}
	for id := range r.BotCasts {
		ids[id] = true
	}
	for id := range r.SimCasts {
		ids[id] = true
	}
	var rows []string
	for _, id := range sortedIDs(ids) {
		bot, simN := r.BotCasts[id], r.SimCasts[id]
		if verbose || botdpsCastsDiffer(bot, simN) {
			rows = append(rows, fmt.Sprintf("    %-32s %6d  bot %5d  sim %7.1f", trim(cmp.Or(r.Names[id], "?"), 32), id, bot, simN))
		}
	}
	if len(rows) > 0 {
		fmt.Fprintln(out, "  casts:")
		fmt.Fprintln(out, strings.Join(rows, "\n"))
	}
	ids = map[int32]bool{}
	for id := range r.BotUptime {
		ids[id] = true
	}
	for id := range r.SimUptime {
		ids[id] = true
	}
	rows = nil
	for _, id := range sortedIDs(ids) {
		bot, simN := r.BotUptime[id], r.SimUptime[id]
		_, botHas := r.BotUptime[id]
		_, simHas := r.SimUptime[id]
		// the bot side has only the auras the bot and its minions put up, so the master's debuffs aren't listed
		if verbose || botHas && (!simHas || math.Abs(bot-simN) > 5) {
			rows = append(rows, fmt.Sprintf("    %-32s %6d  bot %5.1f%%  sim %5.1f%%", trim(cmp.Or(r.Names[id], "?"), 32), id, bot, simN))
		}
	}
	if len(rows) > 0 {
		fmt.Fprintln(out, "  target aura uptime:")
		fmt.Fprintln(out, strings.Join(rows, "\n"))
	}
}

func sortedIDs(ids map[int32]bool) []int32 {
	list := make([]int32, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	slices.Sort(list)
	return list
}

// botdpsFlagPoints is how far below the median bot/sim ratio a build gets bot fixes (the spec-balance PLAN).
const botdpsFlagPoints = 5

func reportBotDpsSummary(out io.Writer, results []botdpsResult) {
	ratios := make([]float64, len(results))
	for i, r := range results {
		ratios[i] = r.ratio()
	}
	sorted := slices.Clone(ratios)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	fmt.Fprintf(out, "\n%-24s %8s %8s %8s\n", "build", "bot", "sim", "bot/sim")
	order := make([]int, len(results))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return ratios[order[a]] < ratios[order[b]] })
	for _, i := range order {
		r := results[i]
		flag := ""
		if ratios[i] < median-botdpsFlagPoints {
			flag = "  more than " + strconv.Itoa(botdpsFlagPoints) + " points below the median"
		}
		fmt.Fprintf(out, "%-24s %8.0f %8.0f %7.1f%%%s\n", r.Run.Build, r.BotDPS, r.SimDPS, ratios[i], flag)
	}
	fmt.Fprintf(out, "median bot/sim %.1f%%\n", median)
}
