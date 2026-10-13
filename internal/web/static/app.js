/* Housecycle dashboard: renders one Chart.js line per series from the embedded JSON.
 * Canvas ids are chartID(indicatorId + "--" + chartId) and must match the template. */
(function () {
  const ACCENT = '#4f6bff';
  const GRID = 'rgba(120, 128, 150, 0.14)';
  const TICK = '#8a90a4';

  function chartID(id) {
    return 'chart-' + String(id).replace(/[^a-zA-Z0-9]+/g, '-');
  }

  function labelFor(iso, freq) {
    const d = new Date(iso);
    if (isNaN(d)) return iso;
    const y = d.getUTCFullYear();
    const m = String(d.getUTCMonth() + 1).padStart(2, '0');
    if (freq === 'W' || freq === 'D') return `${y}-${m}-${String(d.getUTCDate()).padStart(2, '0')}`;
    return `${y}-${m}`;
  }

  function build(canvas, chart) {
    const existing = Chart.getChart(canvas);
    if (existing) existing.destroy();

    const labels = chart.points.map((p) => labelFor(p.date, chart.freq));
    const data = chart.points.map((p) => p.value);
    const gradient = canvas.getContext('2d').createLinearGradient(0, 0, 0, canvas.clientHeight || 190);
    gradient.addColorStop(0, 'rgba(79, 107, 255, 0.20)');
    gradient.addColorStop(1, 'rgba(79, 107, 255, 0.01)');

    const datasets = [{
      label: '',
      data: data,
      borderColor: ACCENT,
      backgroundColor: gradient,
      borderWidth: 2,
      fill: true,
      pointRadius: 0,
      pointHitRadius: 12,
      tension: 0.15,
    }];

    (chart.refs || []).forEach((r) => {
      datasets.push({
        label: r.label,
        data: labels.map(() => r.value),
        borderColor: r.kind === 'parity' ? 'rgba(79, 107, 255, 0.85)' : 'rgba(138, 144, 164, 0.7)',
        borderWidth: r.kind === 'parity' ? 1.5 : 1,
        borderDash: r.kind === 'parity' ? [] : [5, 4],
        pointRadius: 0,
        pointHitRadius: 0,
        fill: false,
        tension: 0,
      });
    });

    new Chart(canvas, {
      type: 'line',
      data: { labels: labels, datasets: datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { display: false },
          tooltip: {
            displayColors: false,
            filter: (item) => item.dataset.label === '',
            callbacks: {
              label: (ctx) => `${ctx.parsed.y} ${chart.unit || ''}`.trim(),
            },
          },
        },
        scales: {
          x: {
            grid: { display: false },
            ticks: { color: TICK, maxRotation: 0, autoSkip: true, maxTicksLimit: 6, font: { size: 10 } },
          },
          y: {
            grid: { color: GRID },
            ticks: { color: TICK, maxTicksLimit: 5, font: { size: 10 } },
          },
        },
      },
    });
  }

  function render() {
    const el = document.getElementById('housecycle-data');
    if (!el) return;
    let data;
    try {
      data = JSON.parse(el.textContent);
    } catch (e) {
      return;
    }
    // Build each chart only when its canvas nears the viewport, so offscreen
    // charts do not block the main thread during load.
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        observer.unobserve(entry.target);
        if (entry.target.__chart) build(entry.target, entry.target.__chart);
      });
    }, { rootMargin: '200px' });

    (data.indicators || []).forEach((ind) => {
      (ind.charts || []).forEach((chart) => {
        const canvas = document.getElementById(chartID(ind.id + '--' + chart.id));
        if (canvas) {
          canvas.__chart = chart;
          observer.observe(canvas);
        }
      });
    });
  }

  document.addEventListener('DOMContentLoaded', render);
})();
