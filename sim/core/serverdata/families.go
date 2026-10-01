package serverdata

import "github.com/wowsims/wotlk/sim/core/proto"

// ClassFamily is the SpellFamilyName of the class's own spells, 0 for an unknown class. Items, racials
// and other classes' spells carry another family.
func ClassFamily(class proto.Class) int32 {
	return classFamilies[class]
}

var classFamilies = map[proto.Class]int32{
	proto.Class_ClassMage: 3, proto.Class_ClassWarrior: 4, proto.Class_ClassWarlock: 5, proto.Class_ClassPriest: 6,
	proto.Class_ClassDruid: 7, proto.Class_ClassRogue: 8, proto.Class_ClassHunter: 9, proto.Class_ClassPaladin: 10,
	proto.Class_ClassShaman: 11, proto.Class_ClassDeathknight: 15,
}
