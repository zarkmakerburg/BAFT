/* BAFT Aurora wizard preview controller. No deployment actions. */
(function () {
 'use strict';
 const stages = ['Topology','Engine','Nodes','Settings','Preview'];
 const engineCatalog = Object.freeze([{id:'baft',label:'BAFT Secure (TLS + Noise)',direct:true,reverse:false,installed:true}]);
 function compatibleEngine(id,topology){const e=engineCatalog.find(x=>x.id===id);return Boolean(e&&e.installed&&(topology==='direct'?e.direct:e.reverse));}
 function validPort(value){return /^[0-9]+$/.test(value)&&Number(value)>=1&&Number(value)<=65535;}
 function validNodes(){return model.iran.trim().length>0&&model.foreign.trim().length>0&&model.iran.trim()!==model.foreign.trim();}
 function canAdvance(stage){return stage===1?compatibleEngine(model.engine,model.topology):stage===2?validNodes():stage===3?validPort(model.port):true;}
 const model = {topology:'direct',engine:'baft',iran:'',foreign:'',port:''};
 const stageNames={fa:['نوع تونل','موتور','سرورها','تنظیمات','پیش‌نمایش'],en:stages};
 const labels={
 fa:{title:'ساخت تونل هوشمند',close:'بستن',back:'قبلی',next:'بعدی',preview:'بستن پیش‌نمایش',topology:'نوع تونل',engine:'موتور تونل',iran:'سرور ایران',foreign:'سرور خارج',port:'پورت عمومی',unsupported:'این موتور از نوع تونل انتخاب‌شده پشتیبانی نمی‌کند.',nodes:'پیش‌نمایش: انتخاب سرور هنوز به فهرست واقعی متصل نیست.',ports:'پورت عمومی از پورت ارتباط بین سرورها مستقل است.',notice:'فقط پیش‌نمایش؛ هیچ تغییری روی سرورها انجام نمی‌شود.'},
 en:{title:'Create Smart Tunnel',close:'Close',back:'Back',next:'Next',preview:'Close preview',topology:'Topology',engine:'Tunnel engine',iran:'Iran node',foreign:'Foreign node',port:'Public ingress port',unsupported:'Selected engine does not support this topology.',nodes:'Preview: live node discovery is not connected.',ports:'Public ingress port is separate from inter-node transport.',notice:'Preview only — no live checks or changes.'}
 };
 function locale(){return document.documentElement.lang==='fa'?'fa':'en';}
 function tr(key){return labels[locale()][key];}
 let active = null, index = 0, returnFocus = null, previousOverflow = null;
 function element(tag,text,cls) {const e=document.createElement(tag);if(text!==undefined)e.textContent=text;if(cls)e.className=cls;return e;}
 function close(){if(!active)return;active.remove();active=null;document.removeEventListener('keydown',onKey);if(previousOverflow!==null){document.body.style.overflow=previousOverflow;previousOverflow=null;}if(returnFocus&&returnFocus.isConnected)returnFocus.focus();}
 function onKey(e){if(!active)return;if(e.key==='Escape'){e.preventDefault();close();return;}if(e.key!=='Tab')return;const a=Array.from(active.querySelectorAll('button,select,input')).filter(x=>!x.disabled);if(!a.length)return;const first=a[0],last=a[a.length-1];if(e.shiftKey&&document.activeElement===first){e.preventDefault();last.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first.focus();}}
 function button(label,fn){const b=element('button',label);b.type='button';b.addEventListener('click',fn);return b;}
 function choice(label,key,options){const wrap=element('label',label+' ');const s=element('select');s.name=key;s.setAttribute('aria-label',label);for(const [value,name] of options){const o=element('option',name);o.value=value;s.append(o);}s.value=model[key];s.addEventListener('change',()=>{model[key]=s.value;render();});wrap.append(s);return wrap;}
 function input(label,key){const wrap=element('label',label+' '),field=element('input');field.name=key;field.setAttribute('aria-label',label);field.value=model[key];if(key==='port'){field.type='number';field.min='1';field.max='65535';field.inputMode='numeric';field.required=true;}field.addEventListener('input',()=>{model[key]=field.value;const next=active?.querySelector('[data-baw-next]');if(next)next.disabled=!canAdvance(index);});wrap.append(field);return wrap;}
 function render(){if(!active)return;const wasInside=active.contains(document.activeElement);const dialog=active.firstElementChild;dialog.replaceChildren();dialog.setAttribute('aria-labelledby','baw-heading');dialog.setAttribute('aria-label',tr('title'));dialog.setAttribute('aria-describedby','baw-content');dialog.setAttribute('dir',document.documentElement.dir||'auto');const header=element('header',undefined,'baw-header');const heading=element('h2',tr('title')+' — '+stageNames[locale()][index],'baw-title');heading.id='baw-heading';header.append(heading,button(tr('close'),close));dialog.append(header);
 const progress=element('div',undefined,'baw-progress');progress.setAttribute('role','group');progress.setAttribute('aria-label',locale()==='fa'?'مراحل ساخت تونل':'Tunnel wizard steps');stages.forEach((name,i)=>{const p=element('span');p.setAttribute('aria-label',stageNames[locale()][i]);p.setAttribute('role','img');if(i===index)p.setAttribute('aria-current','step');if(i<index)p.dataset.complete='true';progress.append(p);});dialog.append(progress);
 const body=element('div',undefined,'baw-body');body.id='baw-content';
 if(index===0)body.append(choice(tr('topology'),'topology',[['direct','Direct'],['reverse','Reverse']]));
 if(index===1){body.append(choice(tr('engine'),'engine',engineCatalog.filter(e=>e.installed).map(e=>[e.id,e.label])));if(!compatibleEngine(model.engine,model.topology)){const warning=element('p',tr('unsupported'));warning.setAttribute('role','alert');body.append(warning);}}
 if(index===2){body.append(element('p',tr('nodes')),input(tr('iran'),'iran'),input(tr('foreign'),'foreign'));}
 if(index===3){body.append(element('p',tr('ports')),input(tr('port'),'port'));}
 if(index===4){for(const [key,value] of Object.entries(model))body.append(element('p',(labels[locale()][key]||key)+': '+(value||'—')));const notice=element('p',tr('notice'));notice.setAttribute('role','status');body.append(notice);}
 dialog.append(body);
 const footer=element('footer',undefined,'baw-actions');const back=button(tr('back'),()=>{index--;render();});back.disabled=index===0;footer.append(back);const next=button(index===4?tr('preview'):tr('next'),()=>{if(index===4)close();else if(canAdvance(index)){index++;render();}});next.disabled=!canAdvance(index);next.dataset.bawNext='true';footer.append(next);dialog.append(footer);if(wasInside){const focusTarget=dialog.querySelector('select,input,button');if(focusTarget)focusTarget.focus();}
 }
 function open(){if(active)return;returnFocus=document.activeElement;index=0;Object.assign(model,{topology:'direct',engine:'baft',iran:'',foreign:'',port:''});previousOverflow=document.body.style.overflow;document.body.style.overflow='hidden';active=element('div',undefined,'baw-backdrop');const dialog=element('section',undefined,'baw-dialog');dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label',tr('title'));
 dialog.setAttribute('dir',document.documentElement.dir||'auto');active.append(dialog);document.body.append(active);active.addEventListener('click',e=>{if(e.target===active)close();});document.addEventListener('keydown',onKey);render();dialog.querySelector('button').focus();}
 window.BAFTActionWizard={open,close,version:'1.0-preview'};
 document.addEventListener('click',event=>{const button=event.target.closest?.('[data-baw-open]');if(button){event.preventDefault();open();}});
 const launch=document.getElementById('baw-open');if(launch){launch.addEventListener('click',open);launch.setAttribute('aria-haspopup','dialog');}
}());
