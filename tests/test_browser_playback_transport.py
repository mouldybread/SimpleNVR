"""Real loopback TLS tooling tests; native media acceptance remains separate."""
import hashlib
import http.client
from pathlib import Path
import ssl
import tempfile
import unittest

from browser_playback_transport import TLSMedia, MAX_MEDIA, MAX_REQUESTS, byte_range, certificate


def get(relay, path, headers=None, method="GET"):
    connection = http.client.HTTPSConnection("127.0.0.1", relay.server.server_address[1],
                                             context=ssl._create_unverified_context(), timeout=3)
    try:
        connection.request(method, path, headers=headers or {})
        return connection, connection.getresponse()
    except BaseException:
        connection.close();raise


class RangeTests(unittest.TestCase):
    def test_supported_ranges_and_clamping(self):
        for value, want in ((None,(0,99,200)),("bytes=0-",(0,99,206)),("bytes=4-8",(4,8,206)),
                            ("bytes=99-999",(99,99,206))):
            self.assertEqual(byte_range(value,100),want)

    def test_invalid_or_unbounded_range_is_refused(self):
        for value in ("", "bytes=-5", "bytes=3-2", "bytes=100-", "bytes=0-2,4-5", "bytes=x-y", "x"*65, True):
            with self.subTest(value=value):
                with self.assertRaises(AssertionError): byte_range(value,100)


class RelayTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.directory=tempfile.TemporaryDirectory(prefix="simplenvr-tls-unit-")
        cls.cert,cls.key=certificate(cls.directory.name)
        cls.media=bytes(range(256))*512

    @classmethod
    def tearDownClass(cls): cls.directory.cleanup()

    def setUp(self):
        self.relay=TLSMedia(self.media,self.cert,self.key);self.relay.start()

    def tearDown(self):
        self.relay.close()
        snapshot=self.relay.snapshot()
        self.assertTrue(snapshot["listener_joined"])
        self.assertEqual(snapshot["active"],0)
        self.assertEqual(snapshot["started"],snapshot["finished"])
        self.assertEqual(snapshot["errors"],[])

    def test_full_body_is_byte_identical(self):
        c,r=get(self.relay,"/control.mp4")
        try:
            self.assertEqual(r.status,200)
            self.assertEqual(int(r.getheader("Content-Length")),len(self.media))
            self.assertEqual(hashlib.sha256(r.read()).digest(),hashlib.sha256(self.media).digest())
        finally: c.close()

    def test_partial_and_head_contract(self):
        c,r=get(self.relay,"/control.mp4",{"Range":"bytes=4-15"})
        try:
            self.assertEqual(r.status,206)
            self.assertEqual(r.getheader("Content-Range"),f"bytes 4-15/{len(self.media)}")
            self.assertEqual(r.read(),self.media[4:16])
        finally: c.close()
        c,r=get(self.relay,"/fault.mp4",method="HEAD")
        try:
            self.assertEqual(r.status,200);self.assertEqual(r.read(),b"")
            self.assertFalse(self.relay.fault_claimed)
        finally: c.close()

    def test_fault_advertises_full_body_then_cuts_after_prefix(self):
        c,r=get(self.relay,"/fault.mp4",{"Range":"bytes=0-"})
        try:
            self.assertEqual(r.status,206)
            self.assertEqual(int(r.getheader("Content-Length")),len(self.media))
            self.assertEqual(r.read(self.relay.prefix),self.media[:self.relay.prefix])
            self.assertTrue(self.relay.prefix_sent.wait(1))
            self.relay.request_cut()
            with self.assertRaises(http.client.IncompleteRead): r.read()
        finally: c.close()
        c,r=get(self.relay,"/fault.mp4")
        try: self.assertEqual((r.status,r.read()),(503,b""))
        finally: c.close()

    def test_stop_releases_blocked_fault_without_claiming_frame_gated_cut(self):
        c,r=get(self.relay,"/fault.mp4")
        try:
            r.read(self.relay.prefix)
            self.assertTrue(self.relay.prefix_sent.wait(1))
            self.relay.close()
            self.assertEqual(self.relay.snapshot()["cut_reason"],"closing")
            self.assertIsNone(self.relay.snapshot()["cut_requested_at"])
        finally: c.close()

    def test_fixed_routes_ranges_and_credentials_not_recorded(self):
        for path,headers,status in (("/etc/passwd",{},404), ("/control.mp4",{"Range":"bytes=-20"},416),
                                    ("/control.mp4?secret=never-log",{"Cookie":"secret-cookie","Authorization":"secret-auth"},200)):
            c,r=get(self.relay,path,headers)
            try: self.assertEqual(r.status,status);r.read()
            finally: c.close()
        snapshot=str(self.relay.snapshot())
        for secret in ("never-log","secret-cookie","secret-auth"): self.assertNotIn(secret,snapshot)

    def test_request_count_ceiling(self):
        for i in range(MAX_REQUESTS):
            c,r=get(self.relay,"/control.mp4",method="HEAD")
            try: self.assertEqual(r.status,200);r.read()
            finally: c.close()
        c=None
        try:
            with self.assertRaises(http.client.RemoteDisconnected):
                c,r=get(self.relay,"/control.mp4",method="HEAD")
        finally:
            if c is not None: c.close()
        snapshot=self.relay.snapshot()
        self.assertEqual(len(snapshot["requests"]),MAX_REQUESTS)
        self.assertEqual(snapshot["excess_requests"],1)

    def test_media_bounds_and_cut_requires_prefix(self):
        for data in (b"", b"x"*65535, b"x"*(MAX_MEDIA+1)):
            with self.assertRaises(AssertionError): TLSMedia(data,self.cert,self.key)
        with self.assertRaises(AssertionError): self.relay.request_cut()


if __name__=="__main__": unittest.main()
