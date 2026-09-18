# Server config values are sim settings, not constants

Module config that changes combat is exposed as sim settings that default to the live config:
mod-dungeon-scale's raid multipliers (live boss health is ×1.2), mod-spell-tweaks' toggles, and
mod-reforging's percentage and stat list. The user tunes the server and wants to judge a change in the
sim before deploying it. Regenerate the defaults whenever the live config changes.
