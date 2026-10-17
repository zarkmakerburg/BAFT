'use strict';
// Runs the landing globe in a small fake browser and counts what it draws.
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const src=fs.readFileSync(path.join(__dirname,'../bcc-welcome-globe-v3.js'),'utf8');

function boot({reduce}){
  let now=0,queue=[],clears=0;const listeners={},mq={matches:reduce,addEventListener:(t,f)=>{mq.change=f}};
  const ctx=new Proxy({},{get:(o,k)=>k==='clearRect'?()=>{clears++}:k==='createRadialGradient'?()=>({addColorStop(){}}):o[k]??(()=>{}),set:(o,k,v)=>{o[k]=v;return true}});
  const canvas={width:0,height:0,style:{},getContext:()=>ctx,getBoundingClientRect:()=>({width:400,height:400}),closest:()=>null,setPointerCapture(){},
    addEventListener:(t,f)=>{listeners[t]=f}};
  const win={devicePixelRatio:1,performance:{now:()=>now},matchMedia:()=>mq,requestAnimationFrame:f=>{queue.push(f);return queue.length},
    addEventListener(){},ResizeObserver:class{observe(){}},MutationObserver:class{observe(){}},IntersectionObserver:class{constructor(f){win.io=f}observe(){}},
    document:{getElementById:()=>canvas,documentElement:{dataset:{theme:'dark'}},hidden:false,addEventListener(){}},Math};
  win.window=win;vm.runInNewContext(src,win);
  const run=ms=>{for(const end=now+ms;now<end;){now+=16;const q=queue;queue=[];q.forEach(f=>f(now))}};
  return {run,get clears(){return clears},get pending(){return queue.length},listeners,mq,win};
}

test('reduced motion: the idle globe stops drawing after its first frame',()=>{
  const g=boot({reduce:true});g.run(2000);
  assert.equal(g.pending,0,'no animation frame should stay scheduled');
  assert.ok(g.clears<=2,'drew '+g.clears+' times while idle');
});
test('reduced motion: a drag still turns the globe, then it settles without gliding',()=>{
  const g=boot({reduce:true});g.run(200);const before=g.clears;
  g.listeners.pointerdown({clientX:100,pointerId:1});
  for(let i=1;i<=5;i++){g.listeners.pointermove({clientX:100+i*20});g.run(16)}
  g.listeners.pointerup();g.run(1000);
  assert.ok(g.clears>before+3,'drag should redraw');
  assert.equal(g.pending,0,'no glide after release with reduced motion');
});
test('normal motion: the globe keeps turning on its own',()=>{
  const g=boot({reduce:false});g.run(2000);
  assert.ok(g.clears>30,'only '+g.clears+' frames in 2s');
  assert.ok(g.pending>0);
});
test('off screen: the globe stops and resumes when visible again',()=>{
  const g=boot({reduce:false});g.run(200);
  g.win.io([{isIntersecting:false}]);g.run(500);const off=g.clears;g.run(1000);
  assert.equal(g.clears,off);assert.equal(g.pending,0);
  g.win.io([{isIntersecting:true}]);g.run(500);
  assert.ok(g.clears>off+10);
});
test('switching reduced motion on at runtime stops the spin',()=>{
  const g=boot({reduce:false});g.run(300);g.mq.matches=true;g.mq.change();g.run(500);
  assert.equal(g.pending,0);
});
