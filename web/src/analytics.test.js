import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
const source=readFileSync(new URL('./analytics.js',import.meta.url),'utf8');
test('reports only public route names and ignores private parameters',()=>{
 const location={hostname:'family.integ.life',hash:'#/feed?member=private-canary'};
 const handlers={},scripts=[];
 const window={history:{pushState(){},replaceState(){}},addEventListener(n,f){handlers[n]=f;}};window.top=window.self=window;
 const document={createElement(){return {};},head:{append(s){scripts.push(s);}}};
 const context=vm.createContext({window,location,document});vm.runInContext(source,context);vm.runInContext(source,context);
 location.hash='#/space';handlers.hashchange();handlers.hashchange();
 const commands=window.dataLayer.map(c=>Array.from(c));
 assert.equal(scripts.length,1);assert.equal(commands.filter(c=>c[1]==='page_view').length,2);
 assert.ok(!JSON.stringify(commands).includes('private-canary'));
 assert.ok(JSON.stringify(commands).includes('https://family.integ.life/space'));
});
test('self-hosted copies do not initialize Google',()=>{vm.runInNewContext(source,{location:{hostname:'localhost'}});});
