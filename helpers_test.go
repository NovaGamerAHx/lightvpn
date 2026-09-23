package main

// Shared fixtures for the tests.

// testPubKey is a REALITY public key (32 bytes, url-safe base64, unpadded) —
// the form share links use.
const testPubKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

// testPubKeyStd is the same key encoded with the standard alphabet (contains +
// and / and padding); the parser must normalise it to the url-safe form.
const testPubKeyStd = "+/+/+/+/+/+/+/+/+/+/+/+/+/+/+/+/+/+/+/+/AAA="

// testPubKeyURL is the same key in the URL-safe alphabet the parser must produce.
const testPubKeyURL = "-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_-_AAA"

const testUUID = "1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d"
