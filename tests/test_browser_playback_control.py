"""Markup, wiring and report regressions; native keyboard proof stays in Chrome."""
import asyncio
import importlib.util
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from browser_playback_control import template_contract, mark_failed_stages

ROOT = Path(__file__).resolve().parent.parent


class MarkupTests(unittest.TestCase):
    def test_actual_template_has_operable_control(self):
        self.assertTrue(template_contract((ROOT / "templates/index.html").read_text()))

    def test_bad_markup_is_rejected(self):
        button = '<button id="play-pause" type="button" aria-label="Play recordings" aria-controls="video-grid" onclick="togglePlayPause()">Play</button>'
        tail = '<div id="video-grid"></div>'
        self.assertTrue(template_contract(button + tail))
        for html in (tail, button+button+tail, button.replace('button','div')+tail,
                     button.replace('type="button"','type="submit"')+tail,
                     button.replace('>Play<','>Pause<')+tail,
                     button.replace('Play recordings','')+tail,
                     button.replace('togglePlayPause()','video.play()')+tail,
                     button.replace('type="button"','type="button" disabled')+tail,
                     button.replace('type="button"','type="button" tabindex="-1"')+tail,
                     button.replace('type="button"','type="button" hidden')+tail,
                     button, button+tail+tail):
            with self.subTest(html=html):
                with self.assertRaises(AssertionError): template_contract(html)

    def test_failure_marks_only_running_stages(self):
        for b,e,want in (("running","not-started",("failed","not-started")),
                         ("passed","running",("passed","failed")),
                         ("passed","passed",("passed","passed"))):
            report={"baseline":b,"playback_edges":e,"status":"passed"}
            mark_failed_stages(report)
            self.assertEqual((report["baseline"],report["playback_edges"]),want)
            self.assertEqual(report["status"],"failed")


class AggregateTests(unittest.IsolatedAsyncioTestCase):
    def module(self):
        spec=importlib.util.spec_from_file_location('browser_expanded_contract',ROOT/'tests/runtime-browser-expanded.py')
        module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
        return module

    async def scenario(self, fail):
        module=self.module();calls=[]
        async def baseline(args):
            calls.append('baseline')
            if fail=='baseline': raise RuntimeError('baseline failed')
        async def edges(args):
            calls.append('edges')
            if fail=='edges': raise RuntimeError('edge failed')
            if fail=='cancel': raise asyncio.CancelledError()
        with tempfile.TemporaryDirectory() as directory:
            args=SimpleNamespace(evidence=Path(directory),commit='a'*40,image_id='fixture')
            with patch.object(module.legacy,'run',baseline),patch.object(module,'run_edges',edges):
                if fail:
                    with self.assertRaises(asyncio.CancelledError if fail=='cancel' else RuntimeError): await module.run(args)
                else: await module.run(args)
            report=json.loads((args.evidence/'runtime-browser-expanded-report.json').read_text())
            for key in ('runtime_qualified','cleanup_authorized'): self.assertIs(report[key],False)
            self.assertEqual(report['commit'],args.commit)
            return report,calls

    async def test_baseline_failure_stops_edge_suite(self):
        report,calls=await self.scenario('baseline')
        self.assertEqual(calls,['baseline'])
        self.assertEqual((report['status'],report['baseline'],report['playback_edges']),('failed','failed','not-started'))

    async def test_edge_failure_preserves_baseline_success(self):
        report,calls=await self.scenario('edges')
        self.assertEqual(calls,['baseline','edges'])
        self.assertEqual((report['status'],report['baseline'],report['playback_edges']),('failed','passed','failed'))

    async def test_cancellation_is_terminal_failure(self):
        report,_=await self.scenario('cancel')
        self.assertEqual((report['status'],report['playback_edges']),('failed','failed'))
        self.assertIn('CancelledError',report['failure'])

    async def test_only_both_stages_pass(self):
        report,calls=await self.scenario(None)
        self.assertEqual(calls,['baseline','edges'])
        self.assertEqual((report['status'],report['baseline'],report['playback_edges']),('passed','passed','passed'))


if __name__=='__main__':
    unittest.main()
