<!-- @format -->
<script>
  import {onMount} from 'svelte'
  import Markdown from 'markdown-it'
  import flua from 'flua'

  function renderLuaSnippets(readme, data) {
    return readme.replace(/{{(.+?)}}/g, (_, code) => {
      try {
        let {res} = flua.runWithGlobals(data, `res = ${code}`, ['res'])
        return res
      } catch (e) {
        return '{' + code + '}'
      }
    })
  }

  const md = Markdown({
    html: false,
    linkify: true,
    typographer: false,
    breaks: true
  })

  export let ctdata = {}
  let id = parseInt(Math.random() * 100000)
  var source
  var target

  onMount(() => {
    source = document.getElementById('c-' + id + '-source')
    target = document.getElementById('c-' + id + '-target')

    target.children[1].innerHTML = md.render(
      renderLuaSnippets(source.children[1].innerHTML, ctdata)
    )
    source.style.display = 'none'

    return () => {
      target.innerHTML = ''
      source.style.display = ''
    }
  })

  function toggle(e) {
    e.preventDefault()
    target.style.display = target.style.display === 'none' ? '' : 'none'
    source.style.display = target.style.display === 'none' ? '' : 'none'
  }
</script>

<div id="c-{id}-source">
  <span class="toggle" on:click={toggle}>↬</span>
  <pre><slot></slot></pre>
</div>
<article id="c-{id}-target">
  <span class="toggle" on:click={toggle}>↫</span>
  <div />
</article>

<style>
  div,
  article {
    position: relative;
  }
  pre {
    white-space: pre-wrap;
  }
  .toggle {
    cursor: pointer;
    position: absolute;
    right: 10px;
    top: 4px;
    text-decoration: none;
    font-size: 1.4rem;
    z-index: 2;
    transition: 200ms linear opacity;
    opacity: 0.6;
  }
  .toggle:hover {
    opacity: 1;
  }
</style>
