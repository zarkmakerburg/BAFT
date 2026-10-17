'use strict';
// Shared by the manual screenshot helper and the isolated CI regression run.
const pages=['dashboard','servers','tunnels','monitoring','security','ssl','backups','updates','finance','settings'];
const runs=[];
for(const [device,viewport] of [['desk',{width:1440,height:960}],['mob',{width:390,height:844}]]){
  for(const lang of ['fa','en'])for(const theme of ['dark','light']){
    runs.push([`${device}-${lang}-${theme}`,viewport,lang,theme,pages]);
  }
}
module.exports={pages,runs};
