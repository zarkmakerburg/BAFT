/* BAFT Aurora wizard preview controller. No deployment actions. */
(function () {
 'use strict';
 const stages = ['Topology','Engine','Nodes','Settings','Preview'];
 const model = {topology:'direct',engine:'native',iran:'',foreign:'',port:''};
 let active = null, index = 0, returnFocus = null;
 function element(tag,text,cls) {const e=document.createElement(tag);if(text!==undefined)e.textContent=text;if(cls)e.className=cls;return e;}
 function close(){if(!active)return;active.remove();active=null;document.removeEventListener('keydown',onKey);if(returnFocus&&returnFocus.isConnected)returnFocus.focus();}
 function onKey(e){if(!active)return;if(e.key==='Escape'){e.preventDefault();close();return;}if(e.key!=='Tab')return;const a=Array.from(active.querySelectorAll('button,select,input')).filter(x=>!x.disabled);if(!a.length)return;const first=a[0],last=a[a.length-1];if(e.shiftKey&&document.activeElement===first){e.preventDefault();last.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first.focus();}}
 function button(label,fn){const b=element('button',label);b.type='button';b.addEventListener('click',fn);return b;}
 function choice(label,key,options){const wrap=element('label',label+' ');const s=element('select');for(const [value,name] of options){const o=element('option',name);o.value=value;s.append(o);}s.value=model[key];s.addEventListener('change',()=>{model[key]=s.value;render();});wrap.append(s);return wrap;}
 function input(label,key){const wrap=element('label',label+' '),field=element('input');field.value=model[key];field.addEventListener('input',()=>{model[key]=field.value;});wrap.append(field);return wrap;}
 function render(){if(!active)return;const dialog=active.firstElementChild;dialog.replaceChildren();const header=element('header',undefined,'baw-header');header.append(element('h2','Create Smart Tunnel — '+stages[index],'baw-title'),button('Close',close));dialog.append(header);
 const progress=element('div',undefined,'baw-progress');stages.forEach((name,i)=>{const p=element('span');p.setAttribute('aria-label',name);if(i===index)p.setAttribute('aria-current','step');if(i<index)p.dataset.complete='true';progress.append(p);});dialog.append(progress);
 const body=element('div',undefined,'baw-body');
 if(index===0)body.append(choice('Topology','topology',[['direct','Direct'],['reverse','Reverse']]));
 if(index===1){body.append(choice('Engine','engine',[['native','BAFT Native']]));if(model.topology==='reverse')body.append(element('p','Reverse capability is not yet verified for this engine.'));}
 if(index===2){body.append(element('p','Preview: node entries are illustrative; live node discovery is not connected.'),input('Iran node','iran'),input('Foreign node','foreign'));}
 if(index===3){body.append(element('p','Public ingress port is separate from inter-node transport and local route ports.'),input('Public ingress port','port'));}
 if(index===4){for(const [key,value] of Object.entries(model))body.append(element('p',key+': '+(value||'Not selected')));body.append(element('p','Preview only — no live checks or changes.'));}
 dialog.append(body);
 const footer=element('footer',undefined,'baw-actions');const back=button('Back',()=>{index--;render();});back.disabled=index===0;footer.append(back);const next=button(index===4?'Close preview':'Next',()=>{if(index===4)close();else{index++;render();}});next.disabled=(index===1&&model.topology==='reverse')||(index===3&&(!/^[0-9]+$/.test(model.port)||Number(model.port)<1||Number(model.port)>65535));footer.append(next);dialog.append(footer);
 }
 function open(){if(active)return;returnFocus=document.activeElement;index=0;Object.assign(model,{topology:'direct',engine:'native',iran:'',foreign:'',port:''});active=element('div',undefined,'baw-backdrop');const dialog=element('section',undefined,'baw-dialog');dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Tunnel wizard preview');active.append(dialog);document.body.append(active);active.addEventListener('click',e=>{if(e.target===active)close();});document.addEventListener('keydown',onKey);render();dialog.querySelector('button').focus();}
 window.BAFTActionWizard={open,close,version:'1.0-preview'};
 const launch=document.getElementById('baw-open');if(launch)launch.addEventListener('click',open);
}());
