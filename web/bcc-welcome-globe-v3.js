/* Landing globe: the bundled NASA Earth turning slowly, with illustrative
   routes from Tehran lifted above the surface. Drag to turn it by hand. */
(()=>{'use strict';const c=document.getElementById('welcomeGlobe');if(!c)return;const ctx=c.getContext('2d');if(!ctx)return;
const land=window.BAFT_LAND||[],reduce=matchMedia('(prefers-reduced-motion: reduce)').matches,rad=Math.PI/180,TILT=.32;
const HUB=[51.39,35.69],NODES=[[8.68,50.11],[4.9,52.37],[-.12,51.51],[28.97,41.01],[-79.38,43.65],[103.82,1.35],[-74,40.7],[139.69,35.69],[55.27,25.2]];
let w=1,h=1,dpr=1,last=0,angle=-.95,spin=reduce?0:2*Math.PI/110000,drag=null,vel=0,prev=0,rafId=0;
function resize(){const b=c.getBoundingClientRect();dpr=Math.min(devicePixelRatio||1,1.75);w=c.width=Math.max(1,Math.round(b.width*dpr));h=c.height=Math.max(1,Math.round(b.height*dpr));draw()}
new ResizeObserver(resize).observe(c);
const light=()=>document.documentElement.dataset.theme==='light';
function geom(){const r=Math.min(w*.41,h*.42),cx=w*.54,cy=h*.51;return{r,cx,cy}}
/* Unit vector for a longitude/latitude, rotated to the view; z>0 faces the viewer. */
function vec(lon,lat){const a=lat*rad,b=lon*rad;return[Math.cos(a)*Math.sin(b),Math.sin(a),Math.cos(a)*Math.cos(b)]}
function view([x,y,z],g,lift=1){const s=Math.sin(angle),k=Math.cos(angle),X=x*k+z*s,Z=-x*s+z*k,Y=y*Math.cos(TILT)-Z*Math.sin(TILT),Z2=y*Math.sin(TILT)+Z*Math.cos(TILT);return{x:g.cx+g.r*X*lift,y:g.cy-g.r*Y*lift,z:Z2,lift}}
const visible=(q,g)=>q.z>0||Math.hypot(q.x-g.cx,q.y-g.cy)>g.r;
function slerp(a,b,t){const d=Math.acos(Math.min(1,a[0]*b[0]+a[1]*b[1]+a[2]*b[2])),s=Math.sin(d)||1;const p=Math.sin((1-t)*d)/s,q=Math.sin(t*d)/s;return[a[0]*p+b[0]*q,a[1]*p+b[1]*q,a[2]*p+b[2]*q,d]}
function draw(time=performance.now()){const g=geom(),L=light();ctx.setTransform(1,0,0,1,0,0);ctx.clearRect(0,0,w,h);
  // Atmosphere glow behind the planet.
  const halo=ctx.createRadialGradient(g.cx,g.cy,g.r*.92,g.cx,g.cy,g.r*1.2);halo.addColorStop(0,L?'rgba(120,170,230,.32)':'rgba(70,130,230,.28)');halo.addColorStop(.35,L?'rgba(150,190,235,.14)':'rgba(60,110,210,.10)');halo.addColorStop(1,'rgba(0,0,0,0)');
  ctx.fillStyle=halo;ctx.fillRect(0,0,w,h);
  const textured=window.BAFT_EARTH?.(ctx,g.cx,g.cy,g.r,-angle,TILT);
  if(!textured){ctx.save();ctx.beginPath();ctx.arc(g.cx,g.cy,g.r,0,Math.PI*2);const s=ctx.createRadialGradient(g.cx-g.r*.4,g.cy-g.r*.5,g.r*.1,g.cx,g.cy,g.r);s.addColorStop(0,L?'#dfe9ef':'#1d2c36');s.addColorStop(1,L?'#a9bcc6':'#03080c');ctx.fillStyle=s;ctx.fill();ctx.clip();
    ctx.fillStyle=L?'#d8ccb2':'#24323b';for(const ring of land){ctx.beginPath();let n=0,on=false;for(const[lon,lat]of ring){const q=view(vec(lon,lat),g);if(q.z>0){n++;on?ctx.lineTo(q.x,q.y):ctx.moveTo(q.x,q.y);on=true}else on=false}if(n>ring.length*.85){ctx.closePath();ctx.fill()}}ctx.restore()}
  // Routes: great circles lifted above the surface, hidden where the planet covers them.
  const hub=vec(...HUB),gold=L?'#c08a2c':'#f2c46d',core=L?'#f7d58f':'#ffe7ad';ctx.lineCap='round';
  NODES.forEach((n,i)=>{const b=vec(...n),steps=48,pts=[];let d=0;for(let s=0;s<=steps;s++){const t=s/steps,v=slerp(hub,b,t);d=v[3];pts.push(view(v,g,1+Math.sin(Math.PI*t)*(.03+d*.055)))}
    let any=false;for(let s=1;s<pts.length;s++){const a=pts[s-1],q=pts[s];if(!visible(a,g)||!visible(q,g))continue;any=true;ctx.beginPath();ctx.moveTo(a.x,a.y);ctx.lineTo(q.x,q.y);ctx.strokeStyle=gold;ctx.globalAlpha=L?.35:.3;ctx.lineWidth=5*dpr;ctx.stroke();ctx.globalAlpha=1;ctx.strokeStyle=core;ctx.lineWidth=1.4*dpr;ctx.stroke()}
    if(!any)return;if(!reduce){const t=((time*.00016)+i*.137)%1,q=pts[Math.round(t*steps)];if(visible(q,g)){ctx.beginPath();ctx.arc(q.x,q.y,2.3*dpr,0,Math.PI*2);ctx.fillStyle=core;ctx.shadowColor=gold;ctx.shadowBlur=12*dpr;ctx.fill();ctx.shadowBlur=0}}
    const e=pts[steps];if(e.z>0){ctx.beginPath();ctx.arc(e.x,e.y,3.4*dpr,0,Math.PI*2);ctx.fillStyle=core;ctx.shadowColor=gold;ctx.shadowBlur=16*dpr;ctx.fill();ctx.shadowBlur=0;ctx.beginPath();ctx.arc(e.x,e.y,8*dpr,0,Math.PI*2);ctx.strokeStyle=gold;ctx.globalAlpha=.6;ctx.lineWidth=1*dpr;ctx.stroke();ctx.globalAlpha=1}});
  const hq=view(hub,g);if(hq.z>0){ctx.beginPath();ctx.arc(hq.x,hq.y,5*dpr,0,Math.PI*2);ctx.fillStyle=core;ctx.shadowColor=gold;ctx.shadowBlur=22*dpr;ctx.fill();ctx.shadowBlur=0;ctx.beginPath();ctx.arc(hq.x,hq.y,12*dpr,0,Math.PI*2);ctx.strokeStyle=gold;ctx.globalAlpha=.55;ctx.stroke();ctx.globalAlpha=1}}
function frame(now){const dt=Math.min(64,now-(prev||now));prev=now;
  if(!drag){if(Math.abs(vel)>1e-6){angle+=vel*dt;vel*=Math.pow(.94,dt/16)}else angle+=spin*dt}
  if(now-last>30&&!c.closest('[hidden]')&&!document.hidden){draw(now);last=now}rafId=requestAnimationFrame(frame)}
c.style.touchAction='pan-y';c.style.cursor='grab';
c.addEventListener('pointerdown',e=>{drag={x:e.clientX,a:angle,t:performance.now(),lx:e.clientX};vel=0;c.style.cursor='grabbing';c.setPointerCapture(e.pointerId)});
c.addEventListener('pointermove',e=>{if(!drag)return;const g=geom(),now=performance.now(),dx=(e.clientX-drag.x)*dpr;angle=drag.a+dx/g.r;vel=((e.clientX-drag.lx)*dpr/g.r)/Math.max(8,now-drag.t);drag.lx=e.clientX;drag.t=now});
const end=()=>{drag=null;c.style.cursor='grab'};c.addEventListener('pointerup',end);c.addEventListener('pointercancel',end);
window.addEventListener('baft:earth-ready',()=>draw());resize();document.addEventListener('visibilitychange',()=>{if(document.hidden)cancelAnimationFrame(rafId);else{prev=0;rafId=requestAnimationFrame(frame)}});rafId=requestAnimationFrame(frame);
})();
