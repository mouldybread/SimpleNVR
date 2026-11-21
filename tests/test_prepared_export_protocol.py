import unittest
from unittest.mock import AsyncMock, patch
from types import SimpleNamespace
from prepared_export_protocol import control, create_ready, job_view, release


def response(status, value=None, raw=b"{}"):
    return SimpleNamespace(status=status, headers={"content-type":"application/json"},
        text=AsyncMock(return_value="response"), body=AsyncMock(return_value=raw), json=AsyncMock(return_value=value))


def view(state="ready"):
    return {"id":"a"*64,"state":state,"bytes":42,"download_url":"/api/exports/download?id="+"a"*64}


class ProtocolTests(unittest.IsolatedAsyncioTestCase):
    async def test_creation_not_retried_on_lost_response(self):
        request=SimpleNamespace(post=AsyncMock(side_effect=OSError("connection lost")))
        with self.assertRaises(OSError):
            await create_ready(request,"https://nvr",{"mode":"copy"})
        self.assertEqual(request.post.await_count,1)

    async def test_queued_preparing_ready(self):
        request=SimpleNamespace(post=AsyncMock(return_value=response(202,view("queued"))),
            get=AsyncMock(side_effect=[response(200,view("preparing")),response(200,view())]))
        with patch("prepared_export_protocol.asyncio.sleep",new=AsyncMock()):
            got=await create_ready(request,"https://nvr",{"mode":"accurate"})
        self.assertEqual(got["state"],"ready")
        self.assertEqual(request.post.await_count,1)
        self.assertEqual(request.get.await_count,2)

    async def test_failure_is_not_ready(self):
        request=SimpleNamespace(post=AsyncMock(return_value=response(202,view("failed"))))
        with self.assertRaises(AssertionError):
            await create_ready(request,"https://nvr",{})

    async def test_wrong_status_and_oversized_control_fail(self):
        for answer in (response(503),response(200,{},b"x"*8193)):
            with self.subTest(status=answer.status):
                with self.assertRaises(AssertionError):
                    await control(SimpleNamespace(get=AsyncMock(return_value=answer)),"https://nvr","/status")

    async def test_release_requires_ack_and_http_404(self):
        for status in (200,404):
            request=SimpleNamespace(post=AsyncMock(return_value=response(204)),get=AsyncMock(return_value=response(status)))
            if status==404:
                await release(request,"https://nvr",view())
            else:
                with self.assertRaises(AssertionError): await release(request,"https://nvr",view())
        bad=SimpleNamespace(post=AsyncMock(return_value=response(503)))
        with self.assertRaises(AssertionError): await release(bad,"https://nvr",view())

    def test_ready_url_and_job_identity_are_bound(self):
        for bad in ({**view(),"download_url":"blob:dead"},{**view(),"id":"bad"},{**view(),"bytes":0}):
            with self.assertRaises(AssertionError):job_view(bad)
        with self.assertRaises(AssertionError):job_view(view(),"b"*64)


if __name__=="__main__":unittest.main()
