import importlib.util
import pathlib
import os
import sys
import tempfile
import unittest
import uuid
from unittest.mock import Mock, patch

import psycopg
from psycopg import sql
from psycopg.conninfo import make_conninfo


def fixture_id(number):
    return uuid.UUID(f"00000000-0000-4000-8000-{number:012d}")


class ResnikTestCase(unittest.TestCase):
    def setUp(self):
        self.wordnet = Mock()
        nltk = self.nltk = Mock()
        corpus = Mock(wordnet=self.wordnet, wordnet_ic=Mock())
        spec = importlib.util.spec_from_file_location(
            "resnik", pathlib.Path(__file__).with_name("resnik.py")
        )
        self.resnik = importlib.util.module_from_spec(spec)
        with patch.dict(sys.modules, {"nltk": nltk, "nltk.corpus": corpus}):
            spec.loader.exec_module(self.resnik)


class CalcWeightsTests(ResnikTestCase):
    def test_import_does_not_download_corpora(self):
        self.nltk.download.assert_not_called()

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


class UUIDArgumentTests(ResnikTestCase):
    def test_invalid_and_nil_user_ids_are_rejected_before_database_access(self):
        for value in ("1", "not-a-uuid", str(uuid.UUID(int=0))):
            with self.subTest(value=value), patch.object(self.resnik, "fetch_interests") as fetch, patch("sys.stderr"):
                with self.assertRaises(SystemExit) as raised:
                    self.resnik.main(["--user-id", value])
                self.assertEqual(raised.exception.code, 2)
                fetch.assert_not_called()


class EnvironmentTests(ResnikTestCase):
    def test_file_configures_database_without_overriding_process_environment(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / ".env"
            path.write_text("DATABASE_URL='postgresql://file@localhost/file'\nPGPASSWORD='literal $password # spaces'\n")
            with patch.object(self.resnik, "ENV_FILE", path), patch.dict(os.environ, {}, clear=True):
                self.resnik.load_environment()
                self.assertEqual(self.resnik.database_url(), "postgresql://file@localhost/file")
                self.assertEqual(os.environ["PGPASSWORD"], "literal $password # spaces")
                os.environ["DATABASE_URL"] = "postgresql://service@localhost/service"
                self.resnik.load_environment()
                self.assertEqual(self.resnik.database_url(), "postgresql://service@localhost/service")

    def test_missing_file_keeps_local_database_default(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "missing.env"
            with patch.object(self.resnik, "ENV_FILE", path), patch.dict(os.environ, {}, clear=True):
                self.resnik.load_environment()
                self.assertEqual(self.resnik.database_url(), self.resnik.DEFAULT_DATABASE_URL)


class UserInterestsTests(ResnikTestCase):
    def setUp(self):
        super().setUp()
        database_url = os.environ.get("TEST_DATABASE_URL")
        if not database_url:
            self.skipTest("set TEST_DATABASE_URL to run PostgreSQL integration tests")
        admin = psycopg.connect(database_url, autocommit=True, connect_timeout=5)
        self.addCleanup(admin.close)
        schema = "test_" + uuid.uuid4().hex
        admin.execute(sql.SQL("CREATE SCHEMA {}").format(sql.Identifier(schema)))
        self.addCleanup(lambda: admin.execute(sql.SQL("DROP SCHEMA {} CASCADE").format(sql.Identifier(schema))))
        self.database_url = make_conninfo(database_url, options=f"-c search_path={schema} -c timezone=UTC")
        environment = patch.dict(os.environ, {"DATABASE_URL": self.database_url})
        environment.start()
        self.addCleanup(environment.stop)

        with psycopg.connect(self.database_url) as conn:
            conn.execute(pathlib.Path(__file__).with_name("schema.sql").read_text())
            with conn.cursor() as cursor:
                cursor.executemany(
                    "INSERT INTO users (id, token, expires_at) VALUES (%s, %s, CURRENT_TIMESTAMP + INTERVAL '1 day')",
                    [(fixture_id(number), str(number) * 64) for number in (1, 2, 3)],
                )
                cursor.executemany(
                    "INSERT INTO interests (id, keyword, anchor) VALUES (%s, %s, %s)",
                    [
                        (fixture_id(1), "science", "science.n.01"),
                        (fixture_id(2), "physics", "physics.n.01"),
                        (fixture_id(3), "sports", "sport.n.01"),
                        (fixture_id(4), "unassigned", "software.n.01"),
                    ],
                )
                cursor.executemany("""
                    INSERT INTO user_interests
                        (user_id, interest_id, weight, is_main, is_active)
                    VALUES (%s, %s, %s, %s, %s)
                """, [
                    (fixture_id(1), fixture_id(1), 0.3, True, True),
                    (fixture_id(1), fixture_id(2), 0.2, False, True),
                    (fixture_id(1), fixture_id(3), 0.9, False, False),
                    (fixture_id(2), fixture_id(1), 0.7, False, True),
                    (fixture_id(2), fixture_id(3), 0.8, True, True),
                ])

    def read_preferences(self):
        with psycopg.connect(self.database_url) as conn:
            return conn.execute("""
                SELECT user_id, interest_id, weight, is_main, is_active
                FROM user_interests
                ORDER BY user_id, interest_id
            """).fetchall()

    def test_fetch_returns_joined_fields_for_active_user_interests(self):
        rows = self.resnik.fetch_interests(fixture_id(1))
        self.assertTrue(all(isinstance(row, dict) for row in rows))
        self.assertEqual({row["keyword"]: dict(row) for row in rows}, {
            "science": {
                "id": fixture_id(1), "keyword": "science", "anchor": "science.n.01",
                "user_id": fixture_id(1), "interest_id": fixture_id(1), "weight": 0.3,
                "is_main": 1, "is_active": 1,
            },
            "physics": {
                "id": fixture_id(2), "keyword": "physics", "anchor": "physics.n.01",
                "user_id": fixture_id(1), "interest_id": fixture_id(2), "weight": 0.2,
                "is_main": 0, "is_active": 1,
            },
        })

    def test_fetch_other_user_returns_only_their_preferences(self):
        rows = self.resnik.fetch_interests(fixture_id(2))
        self.assertEqual({row["keyword"] for row in rows}, {"science", "sports"})
        self.assertTrue(all(row["user_id"] == fixture_id(2) for row in rows))
        science = next(row for row in rows if row["keyword"] == "science")
        self.assertEqual(science["weight"], 0.7)
        self.assertEqual(science["is_main"], 0)

    def test_fetch_user_without_preferences_returns_empty_list(self):
        self.assertEqual(self.resnik.fetch_interests(fixture_id(3)), [])

    def test_fetch_unknown_user_returns_empty_list(self):
        self.assertEqual(self.resnik.fetch_interests(fixture_id(999)), [])

    def test_main_uses_requested_user_and_database(self):
        before = self.read_preferences()
        with patch.object(self.resnik, "calc_weights", return_value={"science": 0.25}) as calculate:
            self.resnik.main(["--user-id", str(fixture_id(2))])
        self.assertTrue(all(row["user_id"] == fixture_id(2) for row in calculate.call_args.args[0]))
        after = self.read_preferences()
        self.assertEqual([row for row in after if row[0] == fixture_id(1)], [row for row in before if row[0] == fixture_id(1)])
        self.assertEqual(next(row[2] for row in after if row[0] == fixture_id(2) and row[1] == fixture_id(1)), 0.25)

    def test_update_commits_weights_only_for_requested_user(self):
        self.resnik.update_weights(fixture_id(1), {"science": 1.0, "physics": 0.5})
        self.assertEqual(self.read_preferences(), [
            (fixture_id(1), fixture_id(1), 1.0, 1, 1),
            (fixture_id(1), fixture_id(2), 0.5, 0, 1),
            (fixture_id(1), fixture_id(3), 0.9, 0, 0),
            (fixture_id(2), fixture_id(1), 0.7, 0, 1),
            (fixture_id(2), fixture_id(3), 0.8, 1, 1),
        ])

    def test_update_empty_weights_leaves_preferences_unchanged(self):
        before = self.read_preferences()
        self.resnik.update_weights(fixture_id(1), {})
        self.assertEqual(self.read_preferences(), before)

    def test_update_missing_preferences_does_not_insert_rows(self):
        before = self.read_preferences()
        self.resnik.update_weights(fixture_id(1), {"unassigned": 1.0, "unknown": 1.0})
        self.resnik.update_weights(fixture_id(999), {"science": 1.0})
        self.assertEqual(self.read_preferences(), before)

    def test_update_rolls_back_entire_batch_on_failure(self):
        before = self.read_preferences()
        with psycopg.connect(self.database_url) as conn:
            conn.execute("""
                CREATE FUNCTION reject_physics_weight() RETURNS trigger AS $$
                BEGIN
                    IF NEW.interest_id = '00000000-0000-4000-8000-000000000002' THEN
                        RAISE EXCEPTION 'reject physics weight' USING ERRCODE = '23514';
                    END IF;
                    RETURN NEW;
                END;
                $$ LANGUAGE plpgsql;
                CREATE TRIGGER reject_physics_weight
                BEFORE UPDATE OF weight ON user_interests
                FOR EACH ROW EXECUTE FUNCTION reject_physics_weight();
            """)
        with self.assertRaises(psycopg.IntegrityError):
            self.resnik.update_weights(fixture_id(1), {"science": 1.0, "physics": 0.5})
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

        interests = self.resnik.fetch_interests(fixture_id(1))
        weights = self.resnik.calc_weights(interests)
        self.assertEqual(weights, {"science": 1.0, "physics": 0.5})
        self.resnik.update_weights(fixture_id(1), weights)
        self.assertEqual(self.read_preferences(), [
            (fixture_id(1), fixture_id(1), 1.0, 1, 1),
            (fixture_id(1), fixture_id(2), 0.5, 0, 1),
            (fixture_id(1), fixture_id(3), 0.9, 0, 0),
            (fixture_id(2), fixture_id(1), 0.7, 0, 1),
            (fixture_id(2), fixture_id(3), 0.8, 1, 1),
        ])


if __name__ == "__main__":
    unittest.main()
