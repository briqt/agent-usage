import importlib.util
import json
import tempfile
import unittest
from datetime import date
from pathlib import Path

MODULE_PATH = Path(__file__).with_name("usage.py")
SPEC = importlib.util.spec_from_file_location("agent_usage_local", MODULE_PATH)
usage = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(usage)


class BillingTests(unittest.TestCase):
    def test_claude_one_hour_cache_price(self):
        price = usage.OFFICIAL_PRICING["claude-opus-4-7"]
        cost = usage.calc_cost(
            6, 426, 11519, 18258, price,
            cache_create_1h=11519,
        )
        self.assertAlmostEqual(cost, 0.134999, places=9)

    def test_pricing_match_is_provider_qualified_and_not_fuzzy(self):
        prices = {
            "gpt-5.5": [9, 0, 0, 0, 0],
            "openai/gpt-5.5": [2, 0, 0, 0, 0],
            "reseller/something-gpt-5-special": [1, 0, 0, 0, 0],
        }
        self.assertEqual(usage.match_pricing("gpt-5.5", prices, "openai")[0], 2)
        self.assertIsNone(usage.match_pricing("gpt-5", prices))

    def test_stats_include_cache_and_output(self):
        records = [{
            "session_id": "s", "input": 6, "output": 426,
            "cache_read": 18258, "cache_create": 11519,
            "cost": 0.134999, "native_cost": 0.2, "priced": True,
        }]
        stats = usage.cmd_stats(records)
        self.assertEqual(stats["total_tokens"], 30209)
        self.assertEqual(stats["total_output_tokens"], 426)
        self.assertAlmostEqual(stats["cache_hit_rate"], 18258 / 29783, places=4)
        self.assertEqual(stats["actual_cost_usd"], 0.2)

    def test_claude_content_blocks_are_deduplicated(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "project" / "session.jsonl"
            path.parent.mkdir()
            entries = []
            for second in (0, 1):
                entries.append({
                    "type": "assistant",
                    "sessionId": "session",
                    "requestId": "request",
                    "timestamp": f"2026-07-01T00:00:0{second}Z",
                    "message": {
                        "id": "message",
                        "model": "claude-opus-4-7",
                        "usage": {
                            "input_tokens": 6,
                            "output_tokens": 426,
                            "cache_read_input_tokens": 18258,
                            "cache_creation_input_tokens": 11519,
                            "cache_creation": {
                                "ephemeral_1h_input_tokens": 11519,
                            },
                        },
                    },
                })
            path.write_text("\n".join(json.dumps(entry) for entry in entries))
            records = usage.scan_claude(
                [directory], date(2026, 7, 1), date(2026, 7, 1))
            self.assertEqual(len(records), 1)
            self.assertEqual(records[0]["cache_create_1h"], 11519)


if __name__ == "__main__":
    unittest.main()
