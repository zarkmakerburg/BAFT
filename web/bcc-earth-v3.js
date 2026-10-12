/* NASA Blue Marble (day), Black Marble (night), ocean mask and relief, lit on a
   sphere with native WebGL. Light theme: a sunlit Earth. Dark theme: the night
   side with city lights and a thin sunlit crescent. All textures are bundled. */
(()=>{'use strict';
const canvas=document.createElement('canvas'),gl=canvas.getContext('webgl',{alpha:true,antialias:false,premultipliedAlpha:true,preserveDrawingBuffer:true});if(!gl)return;
const vert='attribute vec2 p;varying vec2 uv;void main(){uv=p;gl_Position=vec4(p,0.,1.);}';
const frag=`precision mediump float;varying vec2 uv;
uniform sampler2D dayT,nightT,waterT,reliefT;uniform float lon,lat,light,px;
void main(){float d=dot(uv,uv);if(d>1.)discard;
vec3 n=vec3(uv.x,uv.y,sqrt(1.-d));
float y=n.y*cos(lat)+n.z*sin(lat),z=-n.y*sin(lat)+n.z*cos(lat),x=n.x*cos(lon)+z*sin(lon);z=-n.x*sin(lon)+z*cos(lon);
vec2 t=vec2(atan(x,z)/6.2831853+.5,.5-asin(clamp(y,-1.,1.))/3.14159265);
vec3 day=texture2D(dayT,t).rgb,night=texture2D(nightT,t).rgb;float water=texture2D(waterT,t).r,relief=texture2D(reliefT,t).r;
vec3 sun=normalize(mix(vec3(-.95,.32,.05),vec3(-.5,.42,.76),light));
float ndl=dot(n,sun),lit=smoothstep(-.08,.35,ndl);
vec3 col=day*(.06+1.02*lit)*(.92+.16*relief);
vec3 h=normalize(sun+vec3(0.,0.,1.));col+=vec3(1.,.93,.8)*pow(max(dot(n,h),0.),60.)*water*.55*lit;
float grey=dot(night,vec3(.299,.587,.114)),city=smoothstep(.05,.42,grey)*(1.-water);
col+=vec3(1.,.70,.30)*city*1.35*(1.-lit)*(1.-light);
col=mix(col,col*vec3(1.05,1.,.93),.35*light);
float rim=pow(1.-n.z,2.6);vec3 air=mix(vec3(.32,.55,1.),vec3(.55,.75,1.),light);col=mix(col,air,rim*(.25+.55*max(lit,.15*(1.-light))));
float a=smoothstep(1.,1.-px,sqrt(d));gl_FragColor=vec4(col*a,a);}`;
function compile(type,src){const s=gl.createShader(type);gl.shaderSource(s,src);gl.compileShader(s);return gl.getShaderParameter(s,gl.COMPILE_STATUS)?s:null}
const vs=compile(gl.VERTEX_SHADER,vert),fs=compile(gl.FRAGMENT_SHADER,frag);if(!vs||!fs)return;
const prog=gl.createProgram();gl.attachShader(prog,vs);gl.attachShader(prog,fs);gl.linkProgram(prog);if(!gl.getProgramParameter(prog,gl.LINK_STATUS))return;gl.useProgram(prog);
const buf=gl.createBuffer();gl.bindBuffer(gl.ARRAY_BUFFER,buf);gl.bufferData(gl.ARRAY_BUFFER,new Float32Array([-1,-1,1,-1,-1,1,-1,1,1,-1,1,1]),gl.STATIC_DRAW);
const pos=gl.getAttribLocation(prog,'p');gl.enableVertexAttribArray(pos);gl.vertexAttribPointer(pos,2,gl.FLOAT,false,0,0);
const base=(window.BAFT_UI?.base?window.BAFT_UI.base+'v3/':'./')+'assets/';
const names=[['dayT','earth-day.jpg'],['nightT','earth-night-2k.jpg'],['waterT','earth-water.jpg'],['reliefT','earth-relief.jpg']];let loaded=0;
names.forEach(([u,file],i)=>{const tex=gl.createTexture(),img=new Image();gl.activeTexture(gl.TEXTURE0+i);gl.bindTexture(gl.TEXTURE_2D,tex);
  for(const [k,v] of [[gl.TEXTURE_WRAP_S,gl.REPEAT],[gl.TEXTURE_WRAP_T,gl.CLAMP_TO_EDGE],[gl.TEXTURE_MIN_FILTER,gl.LINEAR_MIPMAP_LINEAR],[gl.TEXTURE_MAG_FILTER,gl.LINEAR]])gl.texParameteri(gl.TEXTURE_2D,k,v);
  gl.uniform1i(gl.getUniformLocation(prog,u),i);
  img.onload=()=>{gl.activeTexture(gl.TEXTURE0+i);gl.bindTexture(gl.TEXTURE_2D,tex);gl.texImage2D(gl.TEXTURE_2D,0,gl.RGB,gl.RGB,gl.UNSIGNED_BYTE,img);gl.generateMipmap(gl.TEXTURE_2D);if(++loaded===names.length)window.dispatchEvent(new Event('baft:earth-ready'))};img.src=base+file});
const U=k=>gl.getUniformLocation(prog,k),ulon=U('lon'),ulat=U('lat'),ulight=U('light'),upx=U('px');
window.BAFT_EARTH=(ctx,cx,cy,r,lon,lat)=>{if(loaded<names.length||gl.isContextLost())return false;const size=Math.min(1600,Math.ceil(r*2));
  if(canvas.width!==size){canvas.width=canvas.height=size;gl.viewport(0,0,size,size)}
  gl.clearColor(0,0,0,0);gl.clear(gl.COLOR_BUFFER_BIT);gl.uniform1f(ulon,lon);gl.uniform1f(ulat,lat);gl.uniform1f(upx,2.5/size);
  gl.uniform1f(ulight,document.documentElement.dataset.theme==='light'?1:0);gl.drawArrays(gl.TRIANGLES,0,6);ctx.drawImage(canvas,cx-r,cy-r,r*2,r*2);return true};
})();
