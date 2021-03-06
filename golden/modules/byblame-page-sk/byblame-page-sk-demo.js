import './index';
import '../gold-scaffold-sk';
import { $$ } from 'common-sk/modules/dom';
import { fetchMock } from 'fetch-mock';
import {
  canvaskit,
  fakeNow,
  gm,
  svg,
  trstatus,
} from './demo_data';
import { delay } from '../demo_util';
import { testOnlySetSettings } from '../settings';

testOnlySetSettings({
  title: 'Skia Public',
  defaultCorpus: 'gm',
  baseRepoURL: 'https://skia.googlesource.com/skia.git',
});
$$('gold-scaffold-sk')._render(); // pick up title from settings.

// Set up RPC failure simulation.
const getSimulateRpcFailure = () => sessionStorage.getItem('simulateRpcFailure') === 'true';
const setSimulateRpcFailure = (val) => sessionStorage.setItem('simulateRpcFailure', val);
$$('#simulate-rpc-failure').checked = getSimulateRpcFailure();
$$('#simulate-rpc-failure').addEventListener('change', (e) => {
  setSimulateRpcFailure(e.target.checked);
});

const fakeRpcDelayMillis = 300;

function byBlame(response) {
  if (getSimulateRpcFailure()) {
    return 500; // Fake an internal server error.
  }
  return delay(response, fakeRpcDelayMillis);
}

Date.now = () => fakeNow;

fetchMock.get('/json/byblame?query=source_type%3Dcanvaskit', () => byBlame(canvaskit));
fetchMock.get('/json/byblame?query=source_type%3Dgm', () => byBlame(gm));
fetchMock.get('/json/byblame?query=source_type%3Dsvg', () => byBlame(svg));
fetchMock.get('/json/trstatus', () => {
  if ($$('#simulate-rpc-failure').checked) {
    return 500; // Fake an internal server error.
  }
  return delay(trstatus, fakeRpcDelayMillis);
});

// Create the component after we've had a chance to mock the JSON endpoints.
const page = document.createElement('byblame-page-sk');
$$('gold-scaffold-sk').appendChild(page);
