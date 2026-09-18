package optimizer

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	goproto "google.golang.org/protobuf/proto"
)

// PrepareRequest checks req and deep-copies it. It adds every database the request carries to
// core's in one AddToDatabase call, then strips them: the search builds thousands of characters,
// and each one would otherwise take the database's write lock again.
func PrepareRequest(req *proto.OptimizeGearRequest) (*Request, error) {
	if req.GetBase().GetRaid() == nil {
		return nil, errors.New("request has no base raid")
	}
	req = goproto.Clone(req).(*proto.OptimizeGearRequest)

	targetIndex := int(req.TargetRaidIndex)
	target, err := raidPlayer(req.Base.Raid, targetIndex)
	if err != nil {
		return nil, err
	}

	settings := req.Settings
	if settings == nil {
		settings = &proto.OptimizerSettings{}
	}
	if settings.Workers <= 0 {
		settings.Workers = int32(runtime.GOMAXPROCS(0))
	}
	pool := req.Pool
	if pool == nil {
		pool = &proto.CandidatePool{}
	}

	db := &proto.SimDatabase{}
	for _, party := range req.Base.Raid.Parties {
		for _, player := range party.GetPlayers() {
			if player != nil {
				mergeDatabase(db, player.Database)
				player.Database = nil
			}
		}
	}
	mergeDatabase(db, pool.Database)
	pool.Database = nil
	core.AddToDatabase(db)

	racialTraits := target.RacialTraits
	if racialTraits == proto.Race_RaceUnknown {
		racialTraits = target.Race
	}
	seed, err := LoadoutFromProto(target.Equipment, racialTraits)
	if err != nil {
		return nil, fmt.Errorf("seed gear: %w", err)
	}
	if err := checkLoadout(seed); err != nil {
		return nil, fmt.Errorf("seed gear: %w", err)
	}

	warmStarts := make([]Loadout, len(settings.WarmStarts))
	for i, es := range settings.WarmStarts {
		if warmStarts[i], err = LoadoutFromProto(es, racialTraits); err != nil {
			return nil, fmt.Errorf("warm start %d: %w", i, err)
		}
		if err := checkLoadout(warmStarts[i]); err != nil {
			return nil, fmt.Errorf("warm start %d: %w", i, err)
		}
	}

	if err := checkPool(pool); err != nil {
		return nil, err
	}

	r := &Request{
		Base:           req.Base,
		TargetIndex:    targetIndex,
		Settings:       settings,
		Pool:           pool,
		Seed:           seed,
		WarmStarts:     warmStarts,
		Catalog:        make(map[int32]*proto.CatalogItem, len(pool.CatalogItems)),
		LimitGroups:    make(map[int32]*proto.LimitGroup, len(pool.LimitGroups)),
		MetaConditions: make(map[int32]*proto.MetaGemCondition, len(pool.MetaConditions)),
	}
	for _, item := range pool.CatalogItems {
		r.Catalog[item.Id] = item
	}
	for _, group := range pool.LimitGroups {
		r.LimitGroups[group.Id] = group
	}
	for _, cond := range pool.MetaConditions {
		r.MetaConditions[cond.GemId] = cond
	}
	return r, nil
}

// raidPlayer finds a player the way core.NewRaid numbers them: party index * 5 + position.
func raidPlayer(raid *proto.Raid, index int) (*proto.Player, error) {
	numParties := int(raid.NumActiveParties)
	if numParties == 0 || numParties > len(raid.Parties) {
		numParties = len(raid.Parties)
	}
	if index < 0 || index/5 >= numParties {
		return nil, fmt.Errorf("target raid index %d is outside the raid's %d active parties", index, numParties)
	}
	players := raid.Parties[index/5].GetPlayers()
	if index%5 >= len(players) || players[index%5] == nil || players[index%5].Class == proto.Class_ClassUnknown {
		return nil, fmt.Errorf("target raid index %d is an empty raid slot", index)
	}
	return players[index%5], nil
}

func mergeDatabase(dst, src *proto.SimDatabase) {
	dst.Items = append(dst.Items, src.GetItems()...)
	dst.Enchants = append(dst.Enchants, src.GetEnchants()...)
	dst.Gems = append(dst.Gems, src.GetGems()...)
}

// checkLoadout catches what core.NewItem would panic on.
func checkLoadout(l Loadout) error {
	for slot, c := range l.Items {
		if c.ItemID == 0 {
			continue
		}
		if _, ok := core.LookupItem(c.ItemID); !ok {
			return fmt.Errorf("slot %s: item %d isn't in the database", proto.ItemSlot(slot), c.ItemID)
		}
		for _, gem := range c.Gems {
			if gem == 0 {
				continue
			}
			if _, ok := core.LookupGem(gem); !ok {
				return fmt.Errorf("slot %s: gem %d isn't in the database", proto.ItemSlot(slot), gem)
			}
		}
	}
	return nil
}

// checkPool makes sure the search never picks something the sim can't look up. A missing enchant
// wouldn't panic, it would just silently do nothing.
func checkPool(pool *proto.CandidatePool) error {
	for _, slotPool := range pool.Slots {
		for _, id := range slotPool.ItemIds {
			if _, ok := core.LookupItem(id); !ok {
				return fmt.Errorf("pool slot %s: item %d isn't in the database", slotPool.Slot, id)
			}
		}
		for _, options := range slotPool.EnchantOptions {
			for _, id := range options.EnchantIds {
				if _, ok := core.LookupEnchant(id); !ok {
					return fmt.Errorf("pool slot %s: enchant %d for item %d isn't in the database", slotPool.Slot, id, options.ItemId)
				}
			}
		}
	}
	for _, id := range pool.GemIds {
		if _, ok := core.LookupGem(id); !ok {
			return fmt.Errorf("pool gem %d isn't in the database", id)
		}
	}
	return nil
}
