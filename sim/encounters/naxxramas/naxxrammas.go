package naxxramas

func Register() {
	addPatchwerk25("Naxxrammas 25")
	addKelThuzad25("Naxxrammas 25")
	addThaddius25("Naxxrammas 25")
	addLoatheb25("Naxxrammas 25")

	// TODO: Figure out why this isn't pickable
	//addPatchwerk10("Naxxrammas")
}

// mechanicImmunities is creature_immunities -286, the row of every boss here, 10 and 25 player: the usual
// raid boss mechanics (core.RaidBossMechanicImmunities) without interrupts.
func mechanicImmunities() *uint64 {
	mask := uint64(0x49967EFE)
	return &mask
}
