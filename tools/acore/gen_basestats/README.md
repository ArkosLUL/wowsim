# gen_basestats

Generates `sim/core/base_stats_auto_gen.go` and `ui/core/constants/ratings_auto_gen.ts`: level 80 rating
conversions, class scalars, base stats, attack power formulas and avoidance constants.

```bash
tools/acore/dock.sh run ./tools/acore/gen_basestats
```

Rerun it whenever its inputs change:
- gt*.dbc under `-dbc` (default `/dbc/Clean`, dock.sh's mount of `A:/WOW/dbc`);
- `player_class_stats` and `player_race_stats` through `-dsn` (default `AC_DSN`, else the live DB on
  `host.docker.internal`);
- `Unit.h`, `StatSystem.cpp` and `Player.cpp` under `-ac` (default `/ac`).

It writes nothing when a rating the sim converts with one constant differs between classes or aliases, or
when the classes with mana disagree on spell crit per intellect or mana regen per spirit.

It imports `tools/database`, which imports `sim/core`, so `sim/core` must compile first. To change the
generated files' shape, hand-edit the generated Go, then regenerate.
