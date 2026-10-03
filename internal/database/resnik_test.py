import importlib.util
import pathlib
import sys
import unittest
from unittest.mock import Mock, patch


class CalcWeightsTests(unittest.TestCase):
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


if __name__ == "__main__":
    unittest.main()
