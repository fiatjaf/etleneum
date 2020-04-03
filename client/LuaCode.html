<!-- @format -->

<script>
  import {onMount} from 'svelte'

  import hljs from 'highlight.js/lib/highlight'
  import lua from 'highlight.js/lib/languages/lua'
  hljs.registerLanguage('lua', lua)

  let id = parseInt(Math.random() * 100000)
  var source
  var target

  onMount(() => {
    source = document.getElementById('c-' + id + '-source')
    target = document.getElementById('c-' + id + '-target')

    let highlighted = hljs.highlight('lua', source.innerHTML)
    target.innerHTML = highlighted.value.replace(/\&amp;/g, '&')
    source.style.display = 'none'

    return () => {
      target.innerHTML = ''
      source.style.display = ''
    }
  })
</script>

<style>
  code {
    font-size: 0.8rem;
  }

  .lua {
    padding: 3px 7px;
    background-color: rgba(208, 215, 237, 0.3);
    white-space: pre-wrap;
    transition: 300ms ease-in background-color;
  }
  .lua:hover {
    background-color: white;
  }
</style>

<svelte:head>
  <link rel=stylesheet
  href=https://cdnjs.cloudflare.com/ajax/libs/highlight.js/9.15.10/styles/github.min.css
  />
</svelte:head>
<pre>
  <code id="c-{id}-source">
    <slot></slot>
  </code>
</pre>
<pre class="lua">
 <code id="c-{id}-target">
  </code>
</pre>
