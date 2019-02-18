import requests


def test_application_working(etleneum):
    _, url = etleneum
    r = requests.get(url + "/~/contracts")
    assert r.ok
    assert r.json() == {"ok": True, "value": []}
