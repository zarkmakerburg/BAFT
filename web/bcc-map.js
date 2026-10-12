/* Command Center network map: an equirectangular world with each located node
   at its country point and an arc for every IR -> EX tunnel. Only what BCC
   reports is drawn; a node without a country stays off the map. */
(()=>{'use strict';
const rad=Math.PI/180,reduce=matchMedia('(prefers-reduced-motion: reduce)').matches;
let earth=null,earthReady=false;
function loadEarth(base){if(earth)return;earth=new Image();earth.onload=()=>{earthReady=true;window.dispatchEvent(new Event('baft:map-earth'))};earth.src=base+'assets/earth-night.jpg'}
let landMask=null;
/* Dots on a fixed geographic grid where the land polygons cover the cell. */
function landDots(){if(landMask)return landMask;const W=720,H=360,c=document.createElement('canvas');c.width=W;c.height=H;const x=c.getContext('2d');x.fillStyle='#000';for(const ring of window.BAFT_LAND||[]){x.beginPath();ring.forEach(([lon,lat],i)=>{const px=(lon+180)/360*W,py=(90-lat)/180*H;i?x.lineTo(px,py):x.moveTo(px,py)});x.closePath();x.fill()}
const d=x.getImageData(0,0,W,H).data,pts=[];for(let lat=-58;lat<=80;lat+=1.6)for(let lon=-180;lon<180;lon+=1.6){const px=Math.floor((lon+180)/360*W),py=Math.floor((90-lat)/180*H);if(d[(py*W+px)*4+3]>0)pts.push([lon,lat])}return landMask=pts}
function css(name){return getComputedStyle(document.documentElement).getPropertyValue(name).trim()}
let flagOK=null;
function flagSupported(){if(flagOK!==null)return flagOK;try{const c=document.createElement('canvas');c.width=c.height=16;const x=c.getContext('2d');x.font='14px sans-serif';x.fillText('\u{1F1E9}\u{1F1EA}',0,14);const d=x.getImageData(0,0,16,16).data;let color=false;for(let i=0;i<d.length;i+=4)if(d[i+3]&&(Math.abs(d[i]-d[i+1])>30||Math.abs(d[i+1]-d[i+2])>30)){color=true;break}flagOK=color}catch{flagOK=false}return flagOK}
function flagOf(code){return code&&code.length===2?String.fromCodePoint(...[...code.toUpperCase()].map(c=>0x1F1A5+c.charCodeAt(0))):''}

function create(canvas,opts){
  const ctx=canvas.getContext('2d');loadEarth(opts.assetBase||'./');
  let w=1,h=1,dpr=1,data={nodes:[],links:[]},mode='world',base=null,view=null,placed=[],hover=null,raf=0,last=0;
  function fit(){const pts=data.nodes.filter(n=>Number.isFinite(n.lat)&&Number.isFinite(n.lon));let lonA=-10,lonB=60,latA=15,latB=55;
    if(pts.length){lonA=Math.min(...pts.map(p=>p.lon));lonB=Math.max(...pts.map(p=>p.lon));latA=Math.min(...pts.map(p=>p.lat));latB=Math.max(...pts.map(p=>p.lat))}
    const narrow=w<700,need=Math.max(narrow?70:110,(lonB-lonA)*(narrow?1.35:1.6)),fill=h/(130*1.08),ppd=Math.min(w/need,Math.max(fill,w/(narrow?150:230)));
    const cLon=(lonA+lonB)/2,half=h/2/(ppd*1.08);let cLat=(latA+latB)/2-half*.12;cLat=Math.min(84-half,Math.max(-62+half,cLat));if(half*2>146)cLat=11;
    view={ppd,cLon,cLat}}
  const X=lon=>w/2+(lon-view.cLon)*view.ppd,Y=lat=>h/2-(lat-view.cLat)*view.ppd*1.08;
  function drawBase(){if(!view)return;base=document.createElement('canvas');base.width=Math.round(w*dpr);base.height=Math.round(h*dpr);const b=base.getContext('2d');b.scale(dpr,dpr);
    const light=document.documentElement.dataset.theme==='light';
    b.fillStyle=light?'#efece6':'#05070a';b.fillRect(0,0,w,h);
    if(mode==='world'&&!light&&earthReady){const ww=360*view.ppd,hh=180*view.ppd*1.08,y0=Y(90);b.filter='sepia(.55) saturate(1.5) brightness(.92) contrast(1.08)';for(const k of [-1,0,1])b.drawImage(earth,X(-180)+k*ww,y0,ww,hh);b.filter='none'}
    else if(mode==='world'&&light){b.fillStyle='#e2dccf';b.strokeStyle='#d3cab9';b.lineWidth=.7;for(const ring of window.BAFT_LAND||[]){b.beginPath();ring.forEach(([lon,lat],i)=>{const px=X(lon),py=Y(lat);i?b.lineTo(px,py):b.moveTo(px,py)});b.closePath();b.fill();b.stroke()}}
    else{const dots=landDots(),r=Math.max(.8,view.ppd*.42);b.fillStyle=light?'rgba(120,104,76,.38)':'rgba(232,181,78,.30)';for(const[lon,lat]of dots){const px=X(lon),py=Y(lat);if(px<-4||px>w+4||py<-4||py>h+4)continue;b.beginPath();b.arc(px,py,r,0,Math.PI*2);b.fill()}}
    // Edges fade into the card so the map reads as a curved surface.
    const g=b.createRadialGradient(w*.5,h*.55,Math.min(w,h)*.3,w*.5,h*.55,Math.max(w,h)*.75);g.addColorStop(0,'rgba(0,0,0,0)');g.addColorStop(1,light?'rgba(243,240,234,.92)':'rgba(5,7,10,.88)');b.fillStyle=g;b.fillRect(0,0,w,h);
    const top=b.createLinearGradient(0,0,0,h*.22);top.addColorStop(0,light?'rgba(243,240,234,.95)':'rgba(5,7,10,.9)');top.addColorStop(1,'rgba(0,0,0,0)');b.fillStyle=top;b.fillRect(0,0,w,h*.22);
    const bot=b.createLinearGradient(0,h*.68,0,h);bot.addColorStop(0,'rgba(0,0,0,0)');bot.addColorStop(1,light?'rgba(243,240,234,.96)':'rgba(5,7,10,.94)');b.fillStyle=bot;b.fillRect(0,h*.68,w,h*.32)}
  function resize(){const r=canvas.getBoundingClientRect();dpr=Math.min(devicePixelRatio||1,2);w=Math.max(1,r.width);h=Math.max(1,r.height);canvas.width=Math.round(w*dpr);canvas.height=Math.round(h*dpr);fit();drawBase();draw(performance.now())}
  function positions(){const seen={};placed=[];for(const n of data.nodes){if(!Number.isFinite(n.lat)||!Number.isFinite(n.lon))continue;const key=n.lat+','+n.lon,k=seen[key]=(seen[key]||0)+1;const a=k*2.1,off=k>1?10:0;placed.push({n,x:X(n.lon)+Math.cos(a)*off,y:Y(n.lat)+Math.sin(a)*off,hub:n.role!=='foreign'})}}
  function curve(a,b){const mx=(a.x+b.x)/2,my=(a.y+b.y)/2,dx=b.x-a.x,dy=b.y-a.y,len=Math.hypot(dx,dy)||1;let nx=-dy/len,ny=dx/len;if(ny>0){nx=-nx;ny=-ny}const lift=Math.min(160,len*.28);return{cx:mx+nx*lift,cy:my+ny*lift}}
  function at(a,c,b,t){return{x:(1-t)**2*a.x+2*(1-t)*t*c.cx+t*t*b.x,y:(1-t)**2*a.y+2*(1-t)*t*c.cy+t*t*b.y}}
  function msColor(ms){return ms==null?css('--faint'):ms<120?css('--green'):ms<250?css('--amber'):css('--red')}
  function draw(time){if(!base)return;ctx.setTransform(dpr,0,0,dpr,0,0);ctx.clearRect(0,0,w,h);ctx.drawImage(base,0,0,w,h);positions();
    const light=document.documentElement.dataset.theme==='light',gold=css('--gold'),gold2=light?'#c9952f':'#ffe2a1',red=css('--red'),byId=Object.fromEntries(placed.map(p=>[p.n.id,p]));
    data.links.forEach((l,i)=>{const a=byId[l.ir_node],b=byId[l.ex_node];if(!a||!b)return;const c=curve(a,b),building=l.phase!=='active',down=b.n.status==='down'||a.n.status==='down';
      ctx.beginPath();ctx.moveTo(a.x,a.y);ctx.quadraticCurveTo(c.cx,c.cy,b.x,b.y);
      if(building){ctx.setLineDash([5,7]);ctx.strokeStyle=css('--faint');ctx.lineWidth=1.4;ctx.stroke();ctx.setLineDash([]);return}
      ctx.strokeStyle=down?red:gold;ctx.globalAlpha=light?.25:.35;ctx.lineWidth=6;ctx.shadowColor=down?red:gold;ctx.shadowBlur=light?6:18;ctx.stroke();ctx.globalAlpha=1;ctx.lineWidth=1.6;ctx.strokeStyle=down?red:gold2;ctx.shadowBlur=light?2:10;ctx.stroke();ctx.shadowBlur=0;
      if(!reduce&&!down){const t=((time||0)*.00018+i*.17)%1,p=at(a,c,b,t);ctx.beginPath();ctx.arc(p.x,p.y,2.4,0,Math.PI*2);ctx.fillStyle=gold2;ctx.shadowColor=gold;ctx.shadowBlur=12;ctx.fill();ctx.shadowBlur=0}});
    const boxes=[];
    for(const p of placed){const n=p.n,col=n.status==='up'?gold:n.status==='down'?red:css('--faint');
      if(p.hub){ctx.beginPath();ctx.arc(p.x,p.y,26,0,Math.PI*2);const g=ctx.createRadialGradient(p.x,p.y,4,p.x,p.y,34);g.addColorStop(0,light?'rgba(232,181,78,.35)':'rgba(232,181,78,.30)');g.addColorStop(1,'rgba(232,181,78,0)');ctx.fillStyle=g;ctx.fill();
        ctx.beginPath();ctx.arc(p.x,p.y,18,0,Math.PI*2);ctx.fillStyle=light?'#fffaf0':'#0b0d11';ctx.fill();ctx.lineWidth=2;ctx.strokeStyle=col;ctx.shadowColor=col;ctx.shadowBlur=14;ctx.stroke();ctx.shadowBlur=0;
        ctx.fillStyle=light?'#2a2116':'#fff';ctx.font='700 12px Inter,system-ui,sans-serif';ctx.textAlign='center';ctx.textBaseline='middle';ctx.fillText((n.country||'IR').slice(0,2),p.x,p.y+.5);continue}
      ctx.beginPath();ctx.arc(p.x,p.y,4.5,0,Math.PI*2);ctx.fillStyle=col;ctx.shadowColor=col;ctx.shadowBlur=16;ctx.fill();ctx.shadowBlur=0;ctx.beginPath();ctx.arc(p.x,p.y,9,0,Math.PI*2);ctx.strokeStyle=col;ctx.globalAlpha=.55;ctx.lineWidth=1;ctx.stroke();ctx.globalAlpha=1;
      // Narrow maps get a short tag; the server list underneath carries the details.
      if(w<520){ctx.font='700 10.5px Inter,system-ui,sans-serif';ctx.direction='ltr';ctx.textBaseline='middle';const tag=n.country||n.alias,tw2=ctx.measureText(tag).width,hb=placed.find(q=>q.hub),lf=hb?p.x<hb.x:false,tx=lf?p.x-12-tw2:p.x+12;if(boxes.some(o=>tx-5<o.x+o.w&&tx+tw2+5>o.x&&p.y-9<o.y+18&&p.y+9>o.y))continue;boxes.push({x:tx-5,y:p.y-9,w:tw2+10});ctx.beginPath();ctx.roundRect(tx-5,p.y-9,tw2+10,18,6);ctx.fillStyle=light?'rgba(255,255,255,.85)':'rgba(12,14,19,.75)';ctx.fill();ctx.fillStyle=light?'#1d2128':'#f3efe6';ctx.textAlign='left';ctx.fillText(tag,tx,p.y+.5);continue}
      // Label: flag, country, node name and measured latency, on the side away from the hub.
      const rtl=document.documentElement.dir==='rtl',title=opts.countryName(n.country)||n.alias,sub=n.alias,ms=n.latency_ms!=null?n.latency_ms+' ms':opts.unknown();
      ctx.font='700 12.5px '+css('--font');let tw=ctx.measureText(title).width;ctx.font='500 11px '+css('--font');tw=Math.max(tw,ctx.measureText(sub).width);tw+=46;ctx.font='600 11.5px Inter,system-ui,sans-serif';tw=Math.max(tw,ctx.measureText(ms).width+46);
      const hub=placed.find(q=>q.hub),left=hub?p.x<hub.x:false;let bx=left?p.x-16-tw:p.x+16,by=p.y-24;if(bx+tw>w-8)bx=p.x-16-tw;if(bx<8)bx=p.x+16;by=Math.max(70,Math.min(h-150,by));
      for(let pass=0;pass<4;pass++)for(const o of boxes)if(bx<o.x+o.w&&bx+tw>o.x&&by<o.y+60&&by+60>o.y)by=o.y+62;boxes.push({x:bx,y:by,w:tw});
      if(Math.abs(by+16-p.y)>18){ctx.beginPath();ctx.moveTo(p.x,p.y);ctx.lineTo(left?bx+tw:bx,by+16);ctx.strokeStyle=css('--line2');ctx.lineWidth=1;ctx.stroke()}
      ctx.beginPath();ctx.roundRect(bx-4,by-4,tw+8,56,10);ctx.fillStyle=light?'rgba(255,255,255,.82)':'rgba(12,14,19,.72)';ctx.fill();ctx.strokeStyle=light?'rgba(181,131,42,.25)':'rgba(232,181,78,.18)';ctx.lineWidth=1;ctx.stroke();
      const fx=rtl?bx+tw-16:bx+16,fy=by+24;ctx.beginPath();ctx.arc(fx,fy,14,0,Math.PI*2);ctx.fillStyle=light?'#fff':'#14171d';ctx.fill();ctx.strokeStyle=light?'#ddd5c7':'#2e3442';ctx.stroke();
      ctx.textAlign='center';ctx.textBaseline='middle';if(n.country&&flagSupported()){ctx.font='17px "Apple Color Emoji","Segoe UI Emoji","Noto Color Emoji",sans-serif';ctx.fillText(flagOf(n.country),fx,fy+1)}else{ctx.font='700 9.5px Inter,system-ui,sans-serif';ctx.fillStyle=gold;ctx.fillText(n.country||'—',fx,fy)}
      const lx=rtl?bx+tw-36:bx+36;ctx.textBaseline='alphabetic';ctx.direction=rtl?'rtl':'ltr';ctx.textAlign=rtl?'right':'left';
      ctx.font='700 12.5px '+css('--font');ctx.fillStyle=light?'#1d2128':'#f3efe6';ctx.fillText(title,lx,by+12);ctx.font='500 11px '+css('--font');ctx.fillStyle=css('--muted');ctx.fillText(sub,lx,by+27);
      ctx.direction='ltr';ctx.font='600 11.5px Inter,system-ui,sans-serif';ctx.fillStyle=msColor(n.latency_ms);ctx.fillText(ms,lx,by+42)}
  }
  function frame(t){raf=0;if(!canvas.isConnected||canvas.closest('[hidden]')||document.hidden){return}if(t-last>40){draw(t);last=t}if(!reduce&&data.links.some(l=>l.phase==='active'))raf=requestAnimationFrame(frame)}
  function kick(){if(!raf)raf=requestAnimationFrame(frame)}
  fit();new ResizeObserver(()=>{resize();kick()}).observe(canvas);
  window.addEventListener('baft:map-earth',()=>{drawBase();draw(performance.now())});
  document.fonts?.ready.then(()=>draw(performance.now()));
  canvas.addEventListener('mousemove',ev=>{const r=canvas.getBoundingClientRect(),x=ev.clientX-r.left,y=ev.clientY-r.top,hit=placed.find(p=>Math.hypot(p.x-x,p.y-y)<(p.hub?20:12));if(hit!==hover){hover=hit;opts.onHover?.(hit?.n||null,x,y)}});
  canvas.addEventListener('mouseleave',()=>{hover=null;opts.onHover?.(null)});
  canvas.addEventListener('click',()=>{if(hover)opts.onSelect?.(hover.n)});
  return{set(d){data=d;fit();drawBase();draw(performance.now());kick()},mode(m){mode=m;drawBase();draw(performance.now())},theme(){drawBase();draw(performance.now())},redraw(){draw(performance.now());kick()}};
}
window.BAFT_MAP={create,flagOf,flagSupported};
})();
