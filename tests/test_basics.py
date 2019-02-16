import requests


def test_basics(etleneum):
    r = requests.get(etleneum + "/~/contracts")
    assert r.ok
