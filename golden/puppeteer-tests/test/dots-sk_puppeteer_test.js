const expect = require('chai').expect;
const setUpPuppeteerAndDemoPageServer = require('./util').setUpPuppeteerAndDemoPageServer;
const takeScreenshot = require('./util').takeScreenshot;

describe('dots-sk', () => {
  const testBed = setUpPuppeteerAndDemoPageServer(); // Contains page and baseUrl.

  beforeEach(async () => {
    await testBed.page.goto(`${testBed.baseUrl}/dist/dots-sk.html`);
  });

  it('should render the demo page', async () => {
    // Smoke test.
    expect(await testBed.page.$$('dots-sk')).to.have.length(1);
  });

  describe('screenshots', () => {
    it('no highlighted traces', async () => {
      await testBed.page.setViewport({ width: 300, height: 100 });
      await takeScreenshot(testBed.page, 'dots-sk');
    });

    it('one highlighted trace', async () => {
      await testBed.page.setViewport({ width: 300, height: 100 });

      // Get canvas position.
      const canvas = await testBed.page.$('canvas');
      const boxModel = await canvas.boxModel();
      const x = boxModel.content[0].x; const
        y = boxModel.content[0].y;

      // Hover over the leftmost dot of the first trace.
      await testBed.page.mouse.move(x + 10, y + 10);

      await takeScreenshot(testBed.page, 'dots-sk_highlighted');
    });
  });
});
