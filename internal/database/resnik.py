import argparse
import os
from pathlib import Path
from uuid import UUID
import psycopg
from dotenv import load_dotenv
from psycopg.rows import dict_row

DEFAULT_DATABASE_URL = "postgresql://news_aggregator@127.0.0.1:5432/news_aggregator?sslmode=disable"
NON_MAIN_DECAY = 1.0
ENV_FILE = Path(__file__).resolve().parents[2] / ".env"


def load_environment():
    # The API supplies these values to its child; direct CLI use reads the same file.
    load_dotenv(ENV_FILE, override=False, interpolate=False)


load_environment()
# NLTK reads NLTK_DATA during import, so load .env first for direct CLI use.
from nltk.corpus import wordnet as wn
from nltk.corpus import wordnet_ic


def database_url():
    return os.environ.get("DATABASE_URL") or DEFAULT_DATABASE_URL

def load_information_content():
    # Corpora are installed during setup, never downloaded during a request.
    return wordnet_ic.ic("ic-brown.dat")

def fetch_interests(user_id):
    with psycopg.connect(database_url(), row_factory=dict_row, connect_timeout=5) as conn:
        rows = conn.execute("""
            SELECT
                i.id,
                i.keyword,
                i.anchor,
                ui.user_id,
                ui.interest_id,
                ui.weight,
                ui.is_main,
                ui.is_active
            FROM interests i
            JOIN user_interests ui ON i.id = ui.interest_id
            WHERE ui.is_active = TRUE
            AND ui.user_id = %s
        """,
            (user_id,)
        ).fetchall()

    return rows


def anchor(kw):
    return wn.synset(kw["anchor"])


def calc_weights(interests):
    main_interests = [m for m in interests if m["is_main"] and m["is_active"]]
    main_anchors = {m["keyword"]: anchor(m) for m in main_interests}
    brown_ic = load_information_content() if main_interests else None

    raw = {}
    for kw in interests:
        if not kw["is_active"]:
            continue

        ka = anchor(kw)
        best = 0.0
        for m in main_interests:
            sim = ka.res_similarity(main_anchors[m["keyword"]], brown_ic)
            if sim is not None and sim > best:
                best = sim
        raw[kw["keyword"]] = best

    max_raw = max(raw.values(), default=0.0) or 1.0
    weights = {kw["keyword"]: 0.0 for kw in interests}
    for kw in interests:
        if not kw["is_active"]:
            continue

        if kw["keyword"] in [m["keyword"] for m in main_interests]:
            weights[kw["keyword"]] = 1.0
        else:
            weights[kw["keyword"]] = (raw[kw["keyword"]] / max_raw) * NON_MAIN_DECAY

    return weights


def update_weights(user_id, weights):
    with psycopg.connect(database_url(), connect_timeout=5) as conn:
        with conn.cursor() as cursor:
            cursor.executemany("""
                UPDATE user_interests
                SET weight = %s
                WHERE user_id = %s
                    AND interest_id = (
                        SELECT id FROM interests WHERE keyword = %s
                    )
            """,
                [(weight, user_id, keyword) for keyword, weight in weights.items()],
            )


def main(argv=None):
    parser = argparse.ArgumentParser(description="Recalculate one reader's interest weights")
    parser.add_argument("--user-id", type=UUID, required=True)
    args = parser.parse_args(argv)
    if args.user_id.int == 0:
        parser.error("--user-id must be a non-zero UUID")
    interests = fetch_interests(args.user_id)
    if not interests:
        return

    weights = calc_weights(interests)
    update_weights(args.user_id, weights)


if __name__ == "__main__":
    main()
