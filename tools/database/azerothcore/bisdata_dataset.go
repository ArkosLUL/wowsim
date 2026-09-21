package azerothcore

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

// Subject kinds, as bistooltip_subject.kind stores them and SUBJ sends them.
const (
	BisSubjectRoster = 0
	BisSubjectSpec   = 1
)

// BisSubject is whose BiS a block is: a raider from the roster, or a class spec.
type BisSubject struct {
	ID       int
	Kind     int
	ClassID  int32
	SpecName string
	// roster raiders only; RaidIndex is -1 for a spec
	GUID      uint32
	Name      string
	RaidIndex int
}

type BisDatasetBlock struct {
	SubjectID    int
	ContentPhase int32
	Objective    proto.OptimizerObjective
	Block        BisBlock
	Payload      string
	Checksum     string
}

// ID is the block's id on the wire. The addon derives it the same way from SUBJ's subject id and
// phases.
func (b *BisDatasetBlock) ID() int {
	return b.SubjectID*10 + int(b.ContentPhase)
}

// BisDataset is everything mod-bis-tooltip serves.
type BisDataset struct {
	// changes whenever anything the addon caches does, so it doubles as the cache key
	Version     string
	SimCommit   string
	CatalogDate string
	// the blocks' objectives, e.g. "own" or "own+raid"
	Objective string
	// CompositionFingerprint of the whole roster, healers included; empty without a roster
	Fingerprint string
	ExportedAt  time.Time
	Subjects    []BisSubject
	Blocks      []BisDatasetBlock
	Warnings    []string
}

// BisResult is one optimizer result and whose BiS it is: a raider by roster name, or an addon class
// and spec (BisClassNames, BisSpecNames).
type BisResult struct {
	// where it came from, for messages
	Source string
	Raider string
	Class  string
	Spec   string
	Result *proto.OptimizerResult
}

// BisResultIndex is the file -results points at. File paths are relative to the index.
type BisResultIndex struct {
	Results []struct {
		File   string `json:"file"`
		Raider string `json:"raider,omitempty"`
		Class  string `json:"class,omitempty"`
		Spec   string `json:"spec,omitempty"`
	} `json:"results"`
}

// LoadBisResults reads a result index and the OptimizerResult protojson files it lists.
func LoadBisResults(indexPath string) ([]BisResult, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	var index BisResultIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("%s: %w", indexPath, err)
	}
	var results []BisResult
	for _, entry := range index.Results {
		path := filepath.Join(filepath.Dir(indexPath), entry.File)
		result := &proto.OptimizerResult{}
		if err := readProtoJSON(path, result); err != nil {
			return nil, err
		}
		results = append(results, BisResult{Source: entry.File, Raider: entry.Raider, Class: entry.Class,
			Spec: entry.Spec, Result: result})
	}
	return results, nil
}

// LoadBisBatch reads a raid batch export. Each raider and phase keeps its latest stage.
func LoadBisBatch(path string) ([]BisResult, error) {
	batch := &proto.OptimizerBatchExport{}
	if err := readProtoJSON(path, batch); err != nil {
		return nil, err
	}
	return BisResultsFromBatch(batch), nil
}

func BisResultsFromBatch(batch *proto.OptimizerBatchExport) []BisResult {
	type key struct {
		raider string
		phase  int32
	}
	latest := map[key]*proto.OptimizerBatchEntry{}
	var order []key
	for _, entry := range batch.GetEntries() {
		k := key{entry.GetRaider(), entry.GetContentPhase()}
		if kept, ok := latest[k]; !ok {
			order = append(order, k)
		} else if kept.GetStage() >= entry.GetStage() {
			continue
		}
		latest[k] = entry
	}
	results := make([]BisResult, len(order))
	for i, k := range order {
		entry := latest[k]
		results[i] = BisResult{
			Source: fmt.Sprintf("batch %s phase %d stage %d", entry.GetRaider(), entry.GetContentPhase(), entry.GetStage()),
			Raider: entry.GetRaider(),
			Result: entry.GetResult(),
		}
	}
	return results
}

func readProtoJSON(path string, message googleProto.Message) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, message); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// LoadRoster reads an acraid roster.
func LoadRoster(path string) (*Roster, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var roster Roster
	if err := json.Unmarshal(data, &roster); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if roster.Version != RosterVersion {
		return nil, fmt.Errorf("%s: roster version %d, want %d", path, roster.Version, RosterVersion)
	}
	return &roster, nil
}

// EnchantSpells gives the spell of an enchant effect on an item; ok is false for an effect the sim
// doesn't know.
type EnchantSpells func(effectID, itemID int32) (spellID int32, ok bool)

// SimEnchantSpells reads the sim's item database, e.g. assets/database/db.json. A few effects belong
// to more than one enchant, e.g. the same stats on two slots, so the item's type picks between them.
func SimEnchantSpells(path string) (EnchantSpells, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	db := database.ReadDatabaseFromJson(string(data))
	byEffect := map[int32][]*proto.UIEnchant{}
	for _, enchant := range db.Enchants {
		byEffect[enchant.EffectId] = append(byEffect[enchant.EffectId], enchant)
	}
	for _, enchants := range byEffect {
		slices.SortFunc(enchants, func(a, b *proto.UIEnchant) int { return cmp.Compare(a.SpellId, b.SpellId) })
	}
	return func(effectID, itemID int32) (int32, bool) {
		enchants := byEffect[effectID]
		if len(enchants) == 0 {
			return 0, false
		}
		if item := db.Items[itemID]; item != nil {
			for _, enchant := range enchants {
				if enchant.Type == item.Type || slices.Contains(enchant.ExtraTypes, item.Type) {
					return enchant.SpellId, true
				}
			}
		}
		return enchants[0].SpellId, true
	}, nil
}

// BuildBisBlock takes each filled slot's best item and its runners-up, best first, with the best
// item's enchant, gems and reforge. Anything it has to leave out comes back as a warning.
func BuildBisBlock(result *proto.OptimizerResult, enchants EnchantSpells) (BisBlock, []string) {
	var warnings []string
	alternatives := map[proto.ItemSlot][]*proto.OptimizerSlotAlternative{}
	for _, alt := range result.GetAlternatives() {
		alternatives[alt.GetSlot()] = append(alternatives[alt.GetSlot()], alt)
	}

	var block BisBlock
	for i, item := range result.GetBest().GetEquipment().GetItems() {
		if item.GetId() == 0 {
			continue
		}
		slot := BisSlot{Slot: proto.ItemSlot(i), Items: []int32{item.GetId()}}

		alts := alternatives[slot.Slot]
		slices.SortStableFunc(alts, func(a, b *proto.OptimizerSlotAlternative) int {
			return cmp.Compare(b.GetScoreDelta(), a.GetScoreDelta())
		})
		for _, alt := range alts {
			if len(slot.Items) == BisMaxRanks {
				break
			}
			id := alt.GetItem().GetId()
			if id == 0 || slices.Contains(slot.Items, id) {
				continue
			}
			if len(slot.Items) == 1 {
				slot.Delta, slot.HasDelta = int32(math.Round(-alt.GetScoreDelta())), true
			}
			slot.Items = append(slot.Items, id)
		}

		if effect := item.GetEnchant(); effect != 0 {
			if spell, ok := enchants(effect, item.GetId()); ok && spell != 0 {
				slot.Enchant = spell
			} else {
				warnings = append(warnings, fmt.Sprintf("slot %d: enchant effect %d has no spell in the sim's database, left out", i, effect))
			}
		}
		for _, gem := range item.GetGems() {
			if gem != 0 {
				slot.Gems = append(slot.Gems, gem)
			}
		}
		if reforge := item.GetReforge(); reforge.GetFromStatType() != 0 && reforge.GetToStatType() != 0 {
			slot.ReforgeFrom, slot.ReforgeTo = reforge.GetFromStatType(), reforge.GetToStatType()
		}
		block.Slots = append(block.Slots, slot)
	}
	return block, warnings
}

// BuildBisDataset turns optimizer results into the dataset mod-bis-tooltip serves. roster is needed
// when a result names a raider, and guids maps its character names to character guids. A result
// that can't be used is skipped with a warning; a conflict between results is an error.
func BuildBisDataset(results []BisResult, roster *Roster, guids map[string]uint32, enchants EnchantSpells,
	exportedAt time.Time) (*BisDataset, error) {
	dataset := &BisDataset{ExportedAt: exportedAt.UTC().Truncate(time.Second)}
	warn := func(format string, args ...any) {
		dataset.Warnings = append(dataset.Warnings, fmt.Sprintf(format, args...))
	}

	raiders := map[string]BisSubject{}
	if roster != nil {
		var members []BisCompositionMember
		perSubgroup := map[int32]int{}
		for _, character := range roster.Characters {
			members = append(members, BisCompositionMember{ClassID: character.ClassID, Tree: MainTalentTree(character.Talents)})
			raidIndex := int(character.Subgroup)*5 + perSubgroup[character.Subgroup]
			perSubgroup[character.Subgroup]++
			raiders[character.Name] = BisSubject{Kind: BisSubjectRoster, ClassID: character.ClassID,
				SpecName: RaiderSpecName(character), GUID: guids[character.Name], Name: character.Name, RaidIndex: raidIndex}
		}
		dataset.Fingerprint = CompositionFingerprint(members)
	}

	type subjectKey struct {
		kind  int
		name  string // raider name, or "<class>/<spec>"
		phase int32
	}
	type pending struct {
		subject BisSubject
		result  BisResult
		phase   int32
	}
	var used []pending
	seen := map[subjectKey]string{}
	for _, result := range results {
		r := result.Result
		switch {
		case r == nil:
			warn("%s: no result, skipped", result.Source)
			continue
		case r.GetErrorResult() != "":
			warn("%s: the run failed (%s), skipped", result.Source, r.GetErrorResult())
			continue
		case len(r.GetBest().GetEquipment().GetItems()) == 0:
			warn("%s: no best loadout, skipped", result.Source)
			continue
		}
		phase := r.GetSettings().GetContentPhase()
		if phase < 1 || phase > BisMaxContentPhase {
			warn("%s: content phase %d, skipped", result.Source, phase)
			continue
		}
		if r.GetCancelled() {
			warn("%s: the run was cancelled, so this is the best it found by then", result.Source)
		}

		var subject BisSubject
		var key subjectKey
		switch {
		case result.Raider != "" && (result.Class != "" || result.Spec != ""):
			return nil, fmt.Errorf("%s: names both a raider and a class spec", result.Source)
		case result.Raider == "" && result.Class == "":
			return nil, fmt.Errorf("%s: names neither a raider nor a class spec", result.Source)
		case result.Raider != "":
			if roster == nil {
				return nil, fmt.Errorf("%s: names raider %s, which needs a roster", result.Source, result.Raider)
			}
			raider, ok := raiders[result.Raider]
			if !ok {
				warn("%s: %s isn't in the roster, skipped", result.Source, result.Raider)
				continue
			}
			if raider.GUID == 0 {
				return nil, fmt.Errorf("%s: no character guid for %s", result.Source, result.Raider)
			}
			subject, key = raider, subjectKey{BisSubjectRoster, result.Raider, phase}
		default:
			classID, ok := bisClassID(result.Class)
			if !ok {
				warn("%s: class %q isn't one of the addon's, skipped", result.Source, result.Class)
				continue
			}
			if !slices.Contains(BisSpecNames[classID], result.Spec) {
				warn("%s: spec %q isn't one of the addon's %s specs, skipped", result.Source, result.Spec, result.Class)
				continue
			}
			subject = BisSubject{Kind: BisSubjectSpec, ClassID: classID, SpecName: result.Spec, RaidIndex: -1}
			key = subjectKey{BisSubjectSpec, result.Class + "/" + result.Spec, phase}
		}
		if other, ok := seen[key]; ok {
			return nil, fmt.Errorf("%s and %s are both %s phase %d", other, result.Source, key.name, phase)
		}
		seen[key] = result.Source
		used = append(used, pending{subject, result, phase})
	}
	if len(used) == 0 {
		return nil, errors.New("no usable results")
	}

	// roster raiders first, in raid order like the raid frame, then specs by class and name
	slices.SortStableFunc(used, func(a, b pending) int {
		return cmp.Or(
			cmp.Compare(a.subject.Kind, b.subject.Kind),
			cmp.Compare(a.subject.RaidIndex, b.subject.RaidIndex),
			cmp.Compare(BisClassNames[a.subject.ClassID], BisClassNames[b.subject.ClassID]),
			cmp.Compare(a.subject.SpecName, b.subject.SpecName),
			cmp.Compare(a.phase, b.phase),
		)
	})

	commits, catalogDates := map[string]int{}, map[string]int{}
	objectives := map[proto.OptimizerObjective]bool{}
	for _, p := range used {
		if n := len(dataset.Subjects); n == 0 || !sameBisSubject(dataset.Subjects[n-1], p.subject) {
			p.subject.ID = n + 1
			dataset.Subjects = append(dataset.Subjects, p.subject)
		}
		subject := dataset.Subjects[len(dataset.Subjects)-1]

		block, warnings := BuildBisBlock(p.result.Result, enchants)
		for _, warning := range warnings {
			warn("%s: %s", p.result.Source, warning)
		}
		payload, err := EncodeBlock(block)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.result.Source, err)
		}
		objective := p.result.Result.GetSettings().GetObjective()
		dataset.Blocks = append(dataset.Blocks, BisDatasetBlock{SubjectID: subject.ID, ContentPhase: p.phase,
			Objective: objective, Block: block, Payload: payload, Checksum: BisChecksum(payload)})

		commits[shortSimCommit(p.result.Result.GetSimCommit())]++
		catalogDates[p.result.Result.GetCatalogDate()]++
		objectives[objective] = true
	}

	dataset.SimCommit = mostCommon(commits)
	dataset.CatalogDate = mostCommon(catalogDates)
	if len(commits) > 1 {
		warn("the results come from %d sim commits; the dataset says %s", len(commits), dataset.SimCommit)
	}
	if len(catalogDates) > 1 {
		warn("the results come from %d catalog dates; the dataset says %s", len(catalogDates), dataset.CatalogDate)
	}
	var names []string
	for objective := range objectives {
		names = append(names, bisObjectiveName(objective))
	}
	slices.Sort(names)
	dataset.Objective = strings.Join(names, "+")
	dataset.Version = bisDatasetVersion(dataset)
	return dataset, nil
}

func sameBisSubject(a, b BisSubject) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.ClassID == b.ClassID && a.SpecName == b.SpecName
}

func bisClassID(name string) (int32, bool) {
	for id, className := range BisClassNames {
		if className == name {
			return id, true
		}
	}
	return 0, false
}

func bisObjectiveName(objective proto.OptimizerObjective) string {
	switch objective {
	case proto.OptimizerObjective_OptimizerObjectiveOwnMetrics:
		return "own"
	case proto.OptimizerObjective_OptimizerObjectiveRaidDps:
		return "raid"
	}
	return fmt.Sprintf("objective%d", objective)
}

// shortSimCommit keeps 12 hex digits of a commit and any suffix, such as "-dirty", so META stays short.
func shortSimCommit(commit string) string {
	hash, suffix, _ := strings.Cut(commit, "-")
	if len(hash) > 12 {
		hash = hash[:12]
	}
	if suffix != "" {
		return hash + "-" + suffix
	}
	return hash
}

// mostCommon breaks ties by the smaller value, so the pick doesn't depend on map order.
func mostCommon(counts map[string]int) string {
	best, bestCount := "", 0
	for value, count := range counts {
		if count > bestCount || count == bestCount && value < best {
			best, bestCount = value, count
		}
	}
	return best
}

// bisDatasetVersion hashes everything the addon caches. ExportedAt stays out, so exporting the same
// results again keeps the version and clients don't sync again for nothing.
func bisDatasetVersion(dataset *BisDataset) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%d\n%s\n%s\n%s\n%s\n", BisProtocolVersion, dataset.SimCommit, dataset.CatalogDate,
		dataset.Objective, dataset.Fingerprint)
	for _, s := range dataset.Subjects {
		fmt.Fprintf(hash, "s%d,%d,%d,%s,%d,%s,%d\n", s.ID, s.Kind, s.ClassID, s.SpecName, s.GUID, s.Name, s.RaidIndex)
	}
	for _, b := range dataset.Blocks {
		fmt.Fprintf(hash, "b%d,%d,%s\n", b.SubjectID, b.ContentPhase, b.Payload)
	}
	return hex.EncodeToString(hash.Sum(nil))[:8]
}
