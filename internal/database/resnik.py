import os
import sqlite3
import nltk
from nltk.corpus import wordnet as wn
from nltk.corpus import wordnet_ic

script_dir = os.path.dirname(os.path.abspath(__file__))
DB_NAME = os.path.join(script_dir, "news.db")
NON_MAIN_DECAY = 1.0

nltk.download("wordnet")
nltk.download("wordnet_ic")
brown_ic = wordnet_ic.ic("ic-brown.dat")

def fetch_interests():
    conn = sqlite3.connect(DB_NAME)
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()

    cursor.execute("SELECT id, keyword, is_main, anchor, is_active FROM interests")
    rows = cursor.fetchall()

    if not rows:
        print("No interests found in the database.")
        return None

    conn.close()
    return rows

def anchor(kw):
    return wn.synset(kw['anchor'])

def calc_weights(interests):
    main_interests = [m for m in interests if m['is_main'] and m['is_active']]
    main_anchors = {m['keyword']: anchor(m) for m in main_interests}

    raw = {}
    for kw in interests:
        if not kw['is_active']:
            continue

        ka = anchor(kw)
        best = 0.0
        for m in main_interests:
            sim = ka.res_similarity(main_anchors[m['keyword']], brown_ic)
            if sim is not None and sim > best:
                best = sim
        raw[kw['keyword']] = best

    max_raw = max(raw.values()) or 1.0
    weights = {}
    for kw in interests:
        if not kw['is_active']:
            continue

        if kw['keyword'] in [m['keyword'] for m in main_interests]:
            weights[kw['keyword']] = 1.0
        else:
            weights[kw['keyword']] = (raw[kw['keyword']] / max_raw) * NON_MAIN_DECAY

    return weights

def update_weights(weights):
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()

    for kw, weight in weights.items():
        cursor.execute("UPDATE interests SET weight = ? WHERE keyword = ?", (weight, kw))

    conn.commit()
    conn.close()

def main():
    interests = fetch_interests()
    if interests is None:
        return

    weights = calc_weights(interests)
    update_weights(weights)

if __name__ == "__main__":
    main()
