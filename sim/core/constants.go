package core

import (
	"time"
)

const CharacterLevel = 80

const GCDMin = time.Second * 1
const GCDDefault = time.Millisecond * 1500

// ATTACK_DISPLAY_DELAY: each swing pushes the other hand's timer to at least this.
const attackDisplayDelay = 200 * time.Millisecond

// SPELL_AURA_IGNORE_MELEE_RESET
const auraTypeIgnoreMeleeReset = 272

const DefaultAttackPowerPerDPS = 14.0

const ResilienceRatingPerCritDamageReductionPercent = ResilienceRatingPerCritReductionChance / 2.2

// Updated based on formulas supplied by InDebt on WoWSims Discord
const EnemyAutoAttackAPCoefficient = 1.0 / (14.0 * 177.0)

// A level 83 target's 15 free resistance over 415, which is all the partial
// resists a boss with no resistance stats gets.
const AverageMagicPartialResistMultiplier = 1 - 15.0/415.0

// IDs for items used in core
const (
	ItemIDAtieshMage            = 22589
	ItemIDAtieshWarlock         = 22630
	ItemIDBraidedEterniumChain  = 24114
	ItemIDChainOfTheTwilightOwl = 24121
	ItemIDEyeOfTheNight         = 24116
	ItemIDJadePendantOfBlasting = 20966
	ItemIDTheLightningCapacitor = 28785
)

type Hand bool

const MainHand Hand = true
const OffHand Hand = false
