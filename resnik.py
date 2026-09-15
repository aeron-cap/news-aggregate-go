import sqlite3

import nltk
from nltk.corpus import wordnet as wn
from nltk.corpus import wordnet_ic

DB_NAME = "news.db"

nltk.download("wordnet")
nltk.download("wordnet_ic")
brown_ic = wordnet_ic.ic("ic-brown.dat")

NON_MAIN_DECAY = 1


def fetch_interests():
    conn = sqlite3.connect(DB_NAME)
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()

    cursor.execute(
        "SELECT id, keyword, weight, is_main, anchor, is_active FROM interests WHERE is_active = 1"
    )
    rows = cursor.fetchall()

    if not rows:
        print("No active interests found in the database.")
        return None

    conn.close()
    return rows


def check_synset_exists(interests):
    valid = {s.name() for s in wn.all_synsets()}

    for interest in interests:
        anchor = interest["anchor"]
        if anchor and anchor not in valid:
            raise SystemExit(
                f"Anchor '{anchor}' is not a valid WordNet synset. Fix Anchors."
            )


def anchor(interest):
    anchor_name = interest["anchor"]
    if not anchor_name:
        return None
    return wn.synset(anchor_name)


def calc_weights(interests):
    main_interests = [m for m in interests if m["is_main"]]
    if not main_interests:
        raise SystemExit(
            "No main interests found. At least one main interest is required."
        )

    main_anchors = {}
    for m in main_interests:
        synset = anchor(m)
        if synset:
            main_anchors[m["keyword"]] = synset

    keywords = [kw["keyword"] for kw in interests]
    raw = {}
    for kw in interests:
        kw_anchor = anchor(kw)
        if not kw_anchor:
            continue

        best = 0.0
        for m_synset in main_anchors.values():
            sim = kw_anchor.res_similarity(m_synset, brown_ic)
            if sim is not None and sim > best:
                best = sim
        raw[kw["keyword"]] = best

    if not raw:
        raise SystemExit("No valid keyword anchors found for similarity calculation.")

    max_raw = max(raw.values()) or 1.0
    weights = {}
    for kw in keywords:
        if kw in [m["keyword"] for m in main_interests]:
            weights[kw] = 1.0
        else:
            weights[kw] = (raw.get(kw, 0) / max_raw) * NON_MAIN_DECAY

    return weights


def insert_interests(weights):
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()

    for kw, weight in weights.items():
        cursor.execute(
            "UPDATE interests SET weight = ? WHERE keyword = ?", (weight, kw)
        )

    conn.commit()
    conn.close()


def main():
    interests = fetch_interests()
    if interests is None:
        return

    check_synset_exists(interests)

    weights = calc_weights(interests)
    insert_interests(weights)
    print("Weights calculated and updated successfully.")


if __name__ == "__main__":
    main()
