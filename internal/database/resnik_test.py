import importlib.util
import pathlib
import sqlite3
import sys
import tempfile
import unittest
from contextlib import closing
from unittest.mock import Mock, patch


class ResnikTestCase(unittest.TestCase):
    def setUp(self):
        # Keep these unit tests independent of NLTK downloads and the real database.
        self.wordnet = Mock()
        nltk = Mock()
        corpus = Mock(wordnet=self.wordnet, wordnet_ic=Mock())
        spec = importlib.util.spec_from_file_location(
            "resnik", pathlib.Path(__file__).with_name("resnik.py")
        )
        self.resnik = importlib.util.module_from_spec(spec)
        with patch.dict(sys.modules, {"nltk": nltk, "nltk.corpus": corpus}):
            spec.loader.exec_module(self.resnik)


class CalcWeightsTests(ResnikTestCase):
    def test_all_inactive_interests_have_zero_weight(self):
        weights = self.resnik.calc_weights([
            {"keyword": "science", "is_active": False, "is_main": True},
            {"keyword": "sports", "is_active": False, "is_main": False},
        ])
        self.assertEqual(weights, {"science": 0.0, "sports": 0.0})
        self.wordnet.synset.assert_not_called()

    def test_empty_interests(self):
        self.assertEqual(self.resnik.calc_weights([]), {})

    def test_no_active_main_interests(self):
        weights = self.resnik.calc_weights([
            {"keyword": "science", "anchor": "science.n.01", "is_active": True, "is_main": False},
            {"keyword": "sports", "is_active": False, "is_main": True},
        ])
        self.assertEqual(weights, {"science": 0.0, "sports": 0.0})

    def test_main_and_related_weights(self):
        main = Mock()
        main.res_similarity.return_value = 10.0
        related = Mock()
        related.res_similarity.return_value = 5.0
        self.wordnet.synset.side_effect = {
            "science.n.01": main,
            "physics.n.01": related,
        }.__getitem__
        weights = self.resnik.calc_weights([
            {"keyword": "science", "anchor": "science.n.01", "is_active": True, "is_main": True},
            {"keyword": "physics", "anchor": "physics.n.01", "is_active": True, "is_main": False},
            {"keyword": "sports", "is_active": False, "is_main": False},
        ])
        self.assertEqual(weights, {"science": 1.0, "physics": 0.5, "sports": 0.0})


class UserInterestsTests(ResnikTestCase):
    def setUp(self):
        super().setUp()
        directory = tempfile.TemporaryDirectory(prefix="resnik-test-")
        self.addCleanup(directory.cleanup)
        self.resnik.DB_NAME = str(pathlib.Path(directory.name) / "news.db")

        with closing(sqlite3.connect(self.resnik.DB_NAME)) as conn:
            conn.execute("PRAGMA foreign_keys = ON")
            conn.executescript(pathlib.Path(__file__).with_name("schema.sql").read_text())
            with conn:
                conn.executemany("INSERT INTO users (id) VALUES (?)", [(1,), (2,), (3,)])
                conn.executemany(
                    "INSERT INTO interests (id, keyword, anchor) VALUES (?, ?, ?)",
                    [
                        (1, "science", "science.n.01"),
                        (2, "physics", "physics.n.01"),
                        (3, "sports", "sport.n.01"),
                        (4, "unassigned", "software.n.01"),
                    ],
                )
                conn.executemany("""
                    INSERT INTO user_interests
                        (user_id, interest_id, weight, is_main, is_active)
                    VALUES (?, ?, ?, ?, ?)
                """, [
                    (1, 1, 0.3, 1, 1),
                    (1, 2, 0.2, 0, 1),
                    (1, 3, 0.9, 0, 0),
                    (2, 1, 0.7, 0, 1),
                    (2, 3, 0.8, 1, 1),
                ])

    def read_preferences(self):
        with closing(sqlite3.connect(self.resnik.DB_NAME)) as conn:
            return conn.execute("""
                SELECT user_id, interest_id, weight, is_main, is_active
                FROM user_interests
                ORDER BY user_id, interest_id
            """).fetchall()

    def test_fetch_returns_joined_fields_for_active_user_interests(self):
        rows = self.resnik.fetch_interests(1)
        self.assertTrue(all(isinstance(row, sqlite3.Row) for row in rows))
        self.assertEqual({row["keyword"]: dict(row) for row in rows}, {
            "science": {
                "id": 1, "keyword": "science", "anchor": "science.n.01",
                "user_id": 1, "interest_id": 1, "weight": 0.3,
                "is_main": 1, "is_active": 1,
            },
            "physics": {
                "id": 2, "keyword": "physics", "anchor": "physics.n.01",
                "user_id": 1, "interest_id": 2, "weight": 0.2,
                "is_main": 0, "is_active": 1,
            },
        })

    def test_fetch_other_user_returns_only_their_preferences(self):
        rows = self.resnik.fetch_interests(2)
        self.assertEqual({row["keyword"] for row in rows}, {"science", "sports"})
        self.assertTrue(all(row["user_id"] == 2 for row in rows))
        science = next(row for row in rows if row["keyword"] == "science")
        self.assertEqual(science["weight"], 0.7)
        self.assertEqual(science["is_main"], 0)

    def test_fetch_user_without_preferences_returns_empty_list(self):
        self.assertEqual(self.resnik.fetch_interests(3), [])

    def test_fetch_unknown_user_returns_empty_list(self):
        self.assertEqual(self.resnik.fetch_interests(999), [])

    def test_update_commits_weights_only_for_requested_user(self):
        self.resnik.update_weights(1, {"science": 1.0, "physics": 0.5})
        self.assertEqual(self.read_preferences(), [
            (1, 1, 1.0, 1, 1),
            (1, 2, 0.5, 0, 1),
            (1, 3, 0.9, 0, 0),
            (2, 1, 0.7, 0, 1),
            (2, 3, 0.8, 1, 1),
        ])

    def test_update_empty_weights_leaves_preferences_unchanged(self):
        before = self.read_preferences()
        self.resnik.update_weights(1, {})
        self.assertEqual(self.read_preferences(), before)

    def test_update_missing_preferences_does_not_insert_rows(self):
        before = self.read_preferences()
        self.resnik.update_weights(1, {"unassigned": 1.0, "unknown": 1.0})
        self.resnik.update_weights(999, {"science": 1.0})
        self.assertEqual(self.read_preferences(), before)

    def test_update_rolls_back_entire_batch_on_failure(self):
        before = self.read_preferences()
        with closing(sqlite3.connect(self.resnik.DB_NAME)) as conn:
            conn.executescript("""
                CREATE TRIGGER reject_physics_weight
                BEFORE UPDATE OF weight ON user_interests
                WHEN NEW.interest_id = 2
                BEGIN
                    SELECT RAISE(ABORT, 'reject physics weight');
                END;
            """)
        with self.assertRaises(sqlite3.IntegrityError):
            self.resnik.update_weights(1, {"science": 1.0, "physics": 0.5})
        self.assertEqual(self.read_preferences(), before)

    def test_fetch_calculate_update_pipeline_is_user_scoped(self):
        main = Mock()
        main.res_similarity.return_value = 10.0
        related = Mock()
        related.res_similarity.return_value = 5.0
        self.wordnet.synset.side_effect = {
            "science.n.01": main,
            "physics.n.01": related,
        }.__getitem__

        interests = self.resnik.fetch_interests(1)
        weights = self.resnik.calc_weights(interests)
        self.assertEqual(weights, {"science": 1.0, "physics": 0.5})
        self.resnik.update_weights(1, weights)
        self.assertEqual(self.read_preferences(), [
            (1, 1, 1.0, 1, 1),
            (1, 2, 0.5, 0, 1),
            (1, 3, 0.9, 0, 0),
            (2, 1, 0.7, 0, 1),
            (2, 3, 0.8, 1, 1),
        ])


if __name__ == "__main__":
    unittest.main()
