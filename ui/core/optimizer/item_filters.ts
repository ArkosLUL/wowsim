import { HandType, ItemSlot } from '../proto/common.js';
import { DatabaseFilters, DungeonDifficulty, Expansion, RaidFilterOption, SourceFilterOption, UIItem as Item, UIItem_FactionRestriction } from '../proto/ui.js';
import { AL_CATEGORY_HARD_MODE } from '../proto_utils/utils.js';

export const ARMOR_SLOTS: Array<ItemSlot> = [
	ItemSlot.ItemSlotHead,
	ItemSlot.ItemSlotShoulder,
	ItemSlot.ItemSlotChest,
	ItemSlot.ItemSlotWrist,
	ItemSlot.ItemSlotHands,
	ItemSlot.ItemSlotLegs,
	ItemSlot.ItemSlotWaist,
	ItemSlot.ItemSlotFeet,
];

export const WEAPON_SLOTS: Array<ItemSlot> = [ItemSlot.ItemSlotMainHand, ItemSlot.ItemSlotOffHand];

const DIFFICULTY_SRCS: Partial<Record<SourceFilterOption, DungeonDifficulty>> = {
	[SourceFilterOption.SourceDungeon]: DungeonDifficulty.DifficultyNormal,
	[SourceFilterOption.SourceDungeonH]: DungeonDifficulty.DifficultyHeroic,
	[SourceFilterOption.SourceRaid10]: DungeonDifficulty.DifficultyRaid10,
	[SourceFilterOption.SourceRaid10H]: DungeonDifficulty.DifficultyRaid10H,
	[SourceFilterOption.SourceRaid25]: DungeonDifficulty.DifficultyRaid25,
	[SourceFilterOption.SourceRaid25H]: DungeonDifficulty.DifficultyRaid25H,
};

const HEROIC_TO_NORMAL: Partial<Record<DungeonDifficulty, DungeonDifficulty>> = {
	[DungeonDifficulty.DifficultyHeroic]: DungeonDifficulty.DifficultyNormal,
	[DungeonDifficulty.DifficultyRaid10H]: DungeonDifficulty.DifficultyRaid10,
	[DungeonDifficulty.DifficultyRaid25H]: DungeonDifficulty.DifficultyRaid25,
};

const RAID_IDS: Partial<Record<RaidFilterOption, number>> = {
	[RaidFilterOption.RaidNaxxramas]: 3456,
	[RaidFilterOption.RaidEyeOfEternity]: 4500,
	[RaidFilterOption.RaidObsidianSanctum]: 4493,
	[RaidFilterOption.RaidVaultOfArchavon]: 4603,
	[RaidFilterOption.RaidUlduar]: 4273,
	[RaidFilterOption.RaidTrialOfTheCrusader]: 4722,
	[RaidFilterOption.RaidOnyxiasLair]: 2159,
	[RaidFilterOption.RaidIcecrownCitadel]: 4812,
	[RaidFilterOption.RaidRubySanctum]: 4987,
};

// The gear picker's item filters (faction, sources, raids, armor and weapon types, weapon speeds), as a
// pure function so the optimizer's pool builder applies exactly what the picker shows.
export function filterItemsByFilters<T>(itemData: Array<T>, getItemFunc: (val: T) => Item, slot: ItemSlot, filters: DatabaseFilters): Array<T> {
	const filterItems = (itemData: Array<T>, filterFunc: (item: Item) => boolean) => {
		return itemData.filter(itemElem => filterFunc(getItemFunc(itemElem)));
	};

	if (filters.factionRestriction != UIItem_FactionRestriction.UNSPECIFIED) {
		itemData = filterItems(
			itemData,
			item => item.factionRestriction == filters.factionRestriction || item.factionRestriction == UIItem_FactionRestriction.UNSPECIFIED,
		);
	}

	if (!filters.sources.includes(SourceFilterOption.SourceCrafting)) {
		itemData = filterItems(itemData, item => !item.sources.some(itemSrc => itemSrc.source.oneofKind == 'crafted'));
	}
	if (!filters.sources.includes(SourceFilterOption.SourceQuest)) {
		itemData = filterItems(itemData, item => !item.sources.some(itemSrc => itemSrc.source.oneofKind == 'quest'));
	}

	for (const [srcOptionStr, difficulty] of Object.entries(DIFFICULTY_SRCS)) {
		const srcOption = parseInt(srcOptionStr) as SourceFilterOption;
		if (!filters.sources.includes(srcOption)) {
			itemData = filterItems(
				itemData,
				item => !item.sources.some(itemSrc => itemSrc.source.oneofKind == 'drop' && itemSrc.source.drop.difficulty == difficulty),
			);

			if (difficulty == DungeonDifficulty.DifficultyRaid10H || difficulty == DungeonDifficulty.DifficultyRaid25H) {
				const normalDifficulty = HEROIC_TO_NORMAL[difficulty];
				itemData = filterItems(
					itemData,
					item =>
						!item.sources.some(
							itemSrc =>
								itemSrc.source.oneofKind == 'drop' &&
								itemSrc.source.drop.difficulty == normalDifficulty &&
								itemSrc.source.drop.category == AL_CATEGORY_HARD_MODE,
						),
				);
			}
		}
	}

	if (!filters.raids.includes(RaidFilterOption.RaidVanilla)) {
		itemData = filterItems(itemData, item => item.expansion != Expansion.ExpansionVanilla);
	}
	if (!filters.raids.includes(RaidFilterOption.RaidTbc)) {
		itemData = filterItems(itemData, item => item.expansion != Expansion.ExpansionTbc);
	}
	for (const [raidOptionStr, zoneId] of Object.entries(RAID_IDS)) {
		const raidOption = parseInt(raidOptionStr) as RaidFilterOption;
		if (!filters.raids.includes(raidOption)) {
			itemData = filterItems(itemData, item => !item.sources.some(itemSrc => itemSrc.source.oneofKind == 'drop' && itemSrc.source.drop.zoneId == zoneId));
		}
	}

	if (ARMOR_SLOTS.includes(slot)) {
		itemData = filterItems(itemData, item => {
			if (!filters.armorTypes.includes(item.armorType)) {
				return false;
			}

			return true;
		});
	} else if (WEAPON_SLOTS.includes(slot)) {
		itemData = filterItems(itemData, item => {
			if (!filters.weaponTypes.includes(item.weaponType)) {
				return false;
			}
			if (!filters.oneHandedWeapons && item.handType != HandType.HandTypeTwoHand) {
				return false;
			}
			if (!filters.twoHandedWeapons && item.handType == HandType.HandTypeTwoHand) {
				return false;
			}

			const minSpeed = slot == ItemSlot.ItemSlotMainHand ? filters.minMhWeaponSpeed : filters.minOhWeaponSpeed;
			const maxSpeed = slot == ItemSlot.ItemSlotMainHand ? filters.maxMhWeaponSpeed : filters.maxOhWeaponSpeed;
			if (minSpeed > 0 && item.weaponSpeed < minSpeed) {
				return false;
			}
			if (maxSpeed > 0 && item.weaponSpeed > maxSpeed) {
				return false;
			}

			return true;
		});
	} else if (slot == ItemSlot.ItemSlotRanged) {
		itemData = filterItems(itemData, item => {
			if (!filters.rangedWeaponTypes.includes(item.rangedWeaponType)) {
				return false;
			}

			const minSpeed = filters.minRangedWeaponSpeed;
			const maxSpeed = filters.maxRangedWeaponSpeed;
			if (minSpeed > 0 && item.weaponSpeed < minSpeed) {
				return false;
			}
			if (maxSpeed > 0 && item.weaponSpeed > maxSpeed) {
				return false;
			}

			return true;
		});
	}
	return itemData;
}
