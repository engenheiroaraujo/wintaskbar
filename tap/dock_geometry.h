// Cálculo puro em pixels lógicos; não consulta nem altera o Explorer.
#pragma once
#include <algorithm>
#include <cmath>

namespace tws {
struct DockGeometry { double maxWidth, offset; };

inline DockGeometry FitDock(double barWidth, double trayLeft, double gap,
                            double dockWidth, double baseLeft, double currentMaxWidth) {
    double limit = std::clamp(trayLeft - gap, 0.0, barWidth);
    // O quadro nativo já desconta a Tray durante Measure. Descontá-la também
    // de MaxWidth esconde apps desnecessariamente. Reduzir apenas o excesso
    // da superfície efetivamente medida; a restrição só diminui até a próxima
    // mudança da geometria externa.
    double excess = std::max(0.0, dockWidth - limit);
    double width = std::max(0.0, std::min(barWidth, currentMaxWidth) - excess);
    double visibleWidth = std::min(dockWidth, limit);
    double left = std::clamp(baseLeft, 0.0, std::max(0.0, limit - visibleWidth));
    return {width, left - baseLeft};
}
}
