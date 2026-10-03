import os
import sqlite3
from ast import keyword
from contextlib import closing
import nltk
from nltk.corpus import wordnet as wn
from nltk.corpus import wordnet_ic

script_dir = os.path.dirname(os.path.abspath(__file__))
DB_NAME = os.path.join(script_dir, "news.db")
NON_MAIN_DECAY = 1.0

nltk.download("wordnet")
nltk.download("wordnet_ic")
brown_ic = wordnet_ic.ic("ic-brown.dat")


def fetch_interests(user_id):
    with closing(sqlite3.connect(DB_NAME)) as conn:
        conn.row_factory = sqlite3.Row

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
            WHERE ui.is_active = 1
            AND ui.user_id = ?
        """,
            (user_id,)
        ).fetchall()

    return rows


def anchor(kw):
    return wn.synset(kw["anchor"])


def calc_weights(interests):
    main_interests = [m for m in interests if m["is_main"] and m["is_active"]]
    main_anchors = {m["keyword"]: anchor(m) for m in main_interests}

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
    with closing(sqlite3.connect(DB_NAME)) as conn:
        with conn:
            conn.executemany("""
                UPDATE user_interests
                SET weight = ?
                WHERE user_id = ?
                    AND interest_id = (
                        SELECT id FROM interests WHERE keyword = ?
                    )
            """,
                [(weight, user_id, keyword) for keyword, weight in weights.items()],
            )


def main():
    user_id = 1
    interests = fetch_interests(user_id)
    if interests is None:
        return

    weights = calc_weights(interests)
    update_weights(user_id, weights)


if __name__ == "__main__":
    main()
