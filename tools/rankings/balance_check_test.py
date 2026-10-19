"""python -B -m unittest tools/rankings/balance_check_test.py"""
import json
import sys
import unittest
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import balance_check  # noqa: E402


def rankings(dps_by_key, fights=(180, 300)):
    """Every build gets the same DPS in every phase and fight; a {fight: dps} value sets it per fight."""
    return {'builds': [
        {'key': k, 'name': k, 'phases': [
            {'phase': p, 'fights': {str(f): {'dps': v[f] if isinstance(v, dict) else v} for f in fights}}
            for p in range(1, 6)]}
        for k, v in dps_by_key.items()]}


def run(current, buffed=(), base=None):
    _, flags = balance_check.check(current, frozenset(buffed), base)
    return flags


class BalanceCheckTest(unittest.TestCase):
    def test_all_in_band_passes(self):
        self.assertEqual(run(rankings({'hunter_bm': 1000, 'warrior_arms': 900})), [])

    def test_in_scope_below_b_flags_every_counted_cell(self):
        flags = run(rankings({'hunter_bm': 1000, 'warrior_arms': 849}))
        self.assertEqual(len(flags), 10)
        self.assertIn('84.9% is below B', flags[0])

    def test_60s_is_reported_not_counted(self):
        current = rankings({'hunter_bm': 1000, 'warrior_arms': {60: 500, 180: 900, 300: 900}}, fights=(60, 180, 300))
        report, flags = balance_check.check(current)
        self.assertEqual(flags, [])
        self.assertIn(' 50.0 D', report[-1])

    def test_percent_rounds_like_the_page(self):
        # 84.96% shows as 85.0%, which is B
        self.assertEqual(run(rankings({'hunter_bm': 1000, 'warrior_arms': 849.6})), [])
        # Math.round takes 964.5 up to 96.5%, Python's round() would give 96.4%
        base = rankings({'hunter_bm': 1000, 'mage_fire': 961})
        self.assertEqual(len(run(rankings({'hunter_bm': 1000, 'mage_fire': 964.5}), ['mage_fire'], base)), 10)

    def test_excluded_build_may_sit_below_b_but_not_reach_s(self):
        self.assertEqual(run(rankings({'hunter_bm': 1000, 'smite_priest': 400})), [])
        flags = run(rankings({'hunter_bm': 1000, 'mage_frost': 960}))
        self.assertEqual(len(flags), 10)
        self.assertIn('excluded build at 96.0%', flags[0])

    def test_buffed_build_capped_below_s(self):
        self.assertEqual(run(rankings({'hunter_bm': 1000, 'warrior_arms': 949}), ['warrior_arms']), [])
        flags = run(rankings({'hunter_bm': 1000, 'warrior_arms': 950}), ['warrior_arms'])
        self.assertEqual(len(flags), 10)
        self.assertIn('buffed to 95.0%', flags[0])

    def test_buffed_cell_already_at_s_may_rise_by_the_slack_only(self):
        base = rankings({'hunter_bm': 1000, 'mage_fire': 961})
        self.assertEqual(run(rankings({'hunter_bm': 1000, 'mage_fire': 964}), ['mage_fire'], base), [])
        flags = run(rankings({'hunter_bm': 1000, 'mage_fire': 965}), ['mage_fire'], base)
        self.assertEqual(len(flags), 10)
        self.assertIn('(was 96.1%)', flags[0])

    def test_without_base_a_buffed_build_at_s_flags(self):
        flags = run(rankings({'hunter_bm': 1000, 'mage_fire': 961}), ['mage_fire'])
        self.assertEqual(len(flags), 10)
        self.assertTrue(all('took the top' not in f for f in flags))

    def test_buff_taking_the_top_flags(self):
        base = rankings({'hunter_bm': 1000, 'mage_fire': 999})
        flags = run(rankings({'hunter_bm': 1000, 'mage_fire': 1001}), ['mage_fire'], base)
        self.assertEqual(len(flags), 10)
        self.assertIn('took the top from hunter_bm', flags[0])

    def test_build_that_held_the_top_may_gain_by_the_slack_only(self):
        base = rankings({'hunter_bm': 990, 'mage_fire': 1000})
        self.assertEqual(run(rankings({'hunter_bm': 990, 'mage_fire': 1003}), ['mage_fire'], base), [])
        flags = run(rankings({'hunter_bm': 990, 'mage_fire': 1004}), ['mage_fire'], base)
        self.assertEqual(len(flags), 10)
        self.assertIn('buffed to 100.4% of its old DPS', flags[0])

    def test_missing_result_flags(self):
        current = rankings({'hunter_bm': 1000})
        current['builds'].append({'key': 'warrior_arms', 'name': 'warrior_arms', 'phases': []})
        flags = run(current)
        self.assertEqual(len(flags), 10)
        self.assertIn('no result', flags[0])

    def test_committed_rankings_report_every_build(self):
        data = json.loads(balance_check.DEST.read_text(encoding='utf-8'))
        report, _ = balance_check.check(data)
        for b in data['builds']:
            self.assertTrue(any(line.startswith(b['name']) for line in report), b['name'])
        self.assertLessEqual(balance_check.EXCLUDED, {b['key'] for b in data['builds']})


if __name__ == '__main__':
    unittest.main()
