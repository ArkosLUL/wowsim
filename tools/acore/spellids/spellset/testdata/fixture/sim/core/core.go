package core

type ActionID struct {
	SpellID int32
	ItemID  int32
	Tag     int32
}

func (a ActionID) IsSpellAction(spellID int32) bool { return a.SpellID == spellID }

func TernaryInt32(cond bool, a, b int32) int32 {
	if cond {
		return a
	}
	return b
}

type Stat int

const Strength Stat = 7

// in a cross-package call, only the parameter's name counts
func NewAura(label string, stacks int32, auraID int32) ActionID {
	return ActionID{SpellID: auraID}
}
